package consulta

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/antifraude"
	"github.com/romeroalmeida/motor-credito/internal/bureau"
	"github.com/romeroalmeida/motor-credito/internal/bureau/serpro"
	"github.com/romeroalmeida/motor-credito/internal/dinheiro"
	"github.com/romeroalmeida/motor-credito/internal/politica"
)

type contadorFake struct{ err error }

func (c contadorFake) ConsultasDoc24h(context.Context, []byte) (int, error)       { return 0, c.err }
func (c contadorFake) DocsPorDispositivo24h(context.Context, string) (int, error) { return 0, c.err }

// contadorLento só responde quando o contexto acaba (ou, se ninguém cancelar, depois de 1s e sem erro).
type contadorLento struct{}

func (contadorLento) ConsultasDoc24h(ctx context.Context, _ []byte) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(time.Second):
		return 0, nil
	}
}
func (c contadorLento) DocsPorDispositivo24h(ctx context.Context, _ string) (int, error) {
	return c.ConsultasDoc24h(ctx, nil)
}

// bureauFixo devolve sempre o mesmo resultado.
type bureauFixo struct{ r bureau.Resultado }

func (b bureauFixo) Consultar(context.Context, string) (bureau.Resultado, error) { return b.r, nil }

func servicoTeste(b bureau.Bureau, c antifraude.Contador) *Servico {
	return &Servico{bureau: b, contador: c, chaveHMAC: make([]byte, 32), chaveCifra: make([]byte, 32),
		timeout: 100 * time.Millisecond, agora: func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) }}
}

func compilar(t *testing.T, p politica.Politica) *politica.Compilada {
	t.Helper()
	c, err := politica.Compilar(p)
	if err != nil {
		t.Fatalf("compilar: %v", err)
	}
	return c
}

func fonteFixa(c *politica.Compilada) fontePolitica {
	return func(context.Context) (*PoliticaRef, *politica.Compilada, error) {
		return &PoliticaRef{ID: "pol_x", Versao: 3}, c, nil
	}
}

// politicaQueFalha compila, mas a avaliação quebra em tempo de execução (divisão por zero).
func politicaQueFalha() politica.Politica {
	return politica.Politica{ExpressaoAvancada: true, Faixas: []politica.Faixa{
		{Rotulo: "A", CEL: "100 / (score - score) > 0", TaxaBps: 100, LimiteFixo: 100_000_000},
	}}
}

func req(doc string, valor int64) Requisicao {
	return Requisicao{Documento: doc, Finalidade: FinalidadeConcessao, Pedido: &Pedido{Valor: valor, Prazo: 12}}
}

func TestAvaliarFailClosed(t *testing.T) {
	modelo := compilar(t, politica.ModeloEmprestimoPF())
	casos := []struct {
		nome       string
		req        Requisicao
		contador   contadorFake
		fonte      fontePolitica
		status     string
		motivo     string
		comScore   bool
		comVersao  bool
		comOfertas bool
	}{
		{"aprovado no caminho feliz", req(bureau.CPFScoreAlto, 100_000), contadorFake{}, fonteFixa(modelo),
			"aprovado", "", true, true, true},
		{"dígito inválido", req("123.456.789-00", 100_000), contadorFake{}, fonteFixa(modelo),
			StatusDocumentoInvalido, MotivoDocumentoInvalido, false, false, false},
		{"falecido vira documento_invalido", req(bureau.CPFFalecido, 100_000), contadorFake{}, fonteFixa(modelo),
			StatusDocumentoInvalido, MotivoDocumentoInvalido, false, false, false},
		{"bureau indisponível", req(bureau.CPFIndisponivel, 100_000), contadorFake{}, fonteFixa(modelo),
			StatusNegado, MotivoBureauIndisponivel, false, false, false},
		{"contador com erro", req(bureau.CPFScoreAlto, 100_000), contadorFake{err: errors.New("banco fora")}, fonteFixa(modelo),
			StatusNegado, MotivoAntifraudeIndisponivel, true, false, false},
		{"sem política publicada", req(bureau.CPFScoreAlto, 100_000), contadorFake{},
			func(context.Context) (*PoliticaRef, *politica.Compilada, error) {
				return nil, nil, errors.New("nunca publicada")
			},
			StatusNegado, MotivoPoliticaIndisponivel, true, false, false},
		{"erro ao avaliar a política", req(bureau.CPFScoreAlto, 100_000), contadorFake{}, fonteFixa(compilar(t, politicaQueFalha())),
			StatusNegado, MotivoErroPolitica, true, true, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			av := servicoTeste(bureau.NovoSimulado(0), c.contador).avaliar(context.Background(), c.req, c.fonte)
			r := av.reg
			if r.Status != c.status {
				t.Fatalf("status = %q, quer %q (motivos %v)", r.Status, c.status, r.Motivos)
			}
			if c.motivo != "" && (len(r.Motivos) == 0 || r.Motivos[len(r.Motivos)-1].Codigo != c.motivo) {
				t.Fatalf("motivos = %v, quer %s", r.Motivos, c.motivo)
			}
			if (r.Score != nil) != c.comScore {
				t.Fatalf("score = %v, quer presente=%v", r.Score, c.comScore)
			}
			if (r.Politica != nil) != c.comVersao {
				t.Fatalf("política = %v, quer presente=%v", r.Politica, c.comVersao)
			}
			if (r.Oferta != nil) != c.comOfertas {
				t.Fatalf("oferta = %v, quer presente=%v", r.Oferta, c.comOfertas)
			}
		})
	}
}

func TestBureauLentoNegaNoTempoMaximo(t *testing.T) {
	s := servicoTeste(bureau.NovoSimulado(0), contadorFake{})
	inicio := time.Now()
	av := s.avaliar(context.Background(), req(bureau.CPFLento, 100_000), fonteFixa(compilar(t, politica.ModeloEmprestimoPF())))
	if decorrido := time.Since(inicio); decorrido > time.Second {
		t.Fatalf("demorou %v; o timeout era %v", decorrido, s.timeout)
	}
	if av.reg.Status != StatusNegado || av.reg.Motivos[0].Codigo != MotivoBureauIndisponivel {
		t.Fatalf("decisão = %s %v", av.reg.Status, av.reg.Motivos)
	}
}

func TestIdade(t *testing.T) {
	agora := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	casos := []struct {
		nasc  string
		idade int
		ok    bool
	}{
		{"2008-11-07", 18, true}, {"2008-11-08", 17, true}, {"2008-10-08", 18, true}, {"2027-01-01", 0, false}, {"", 0, false},
	}
	for _, c := range casos {
		idade, ok := idadeEm(c.nasc, agora)
		if idade != c.idade || ok != c.ok {
			t.Errorf("idadeEm(%q) = %d,%v; quer %d,%v", c.nasc, idade, ok, c.idade, c.ok)
		}
	}
}

func TestValidar(t *testing.T) {
	renda := int64(-1)
	r := Requisicao{Documento: "", Finalidade: "outra", Pedido: &Pedido{Valor: 0, Prazo: 421},
		Dados: Dados{Nascimento: "07/11/1990", Renda: &renda, Email: "x", IP: "999.1.1.1"}}
	campos := r.Validar()
	for _, c := range []string{"documento", "finalidade", "pedido.valor", "pedido.prazo", "dados.nascimento", "dados.renda", "dados.email", "dados.ip"} {
		if _, ok := campos[c]; !ok {
			t.Errorf("faltou erro em %s (%v)", c, campos)
		}
	}
	if _, ok := req(bureau.CPFScoreAlto, dinheiro.ValorMaximo+1).Validar()["pedido.valor"]; !ok {
		t.Error("pedido.valor acima do teto aceito")
	}
	if campos := req(bureau.CPFScoreAlto, dinheiro.ValorMaximo).Validar(); len(campos) != 0 {
		t.Errorf("pedido no teto recusado: %v", campos)
	}
	if campos := req(bureau.CPFScoreAlto, 1).Validar(); len(campos) != 0 {
		t.Errorf("requisição válida recusada: %v", campos)
	}
}

func motivoFinal(r registro) string {
	if len(r.Motivos) == 0 {
		return ""
	}
	return r.Motivos[len(r.Motivos)-1].Codigo
}

// Teste fraco 8 e achado 8: o nascimento VÁLIDO do bureau prevalece sobre o informado (menor que se diz adulto
// cai no MENOR_IDADE); nascimento do bureau ausente ou inválido cai para o informado. Agora = 2026-11-07.
func TestIdadePeloBureau(t *testing.T) {
	modelo := compilar(t, politica.ModeloEmprestimoPF())
	casos := []struct {
		nome                string
		doBureau, informado string
		menor               bool
	}{
		{"bureau menor, informado adulto: vale o bureau", "2010-01-01", "1980-01-01", true},
		{"bureau adulto, informado menor: vale o bureau", "1980-01-01", "2010-01-01", false},
		{"bureau faz 18 hoje", "2008-11-07", "2010-01-01", false},
		{"bureau faz 18 amanhã", "2008-11-08", "1980-01-01", true},
		{"bureau ausente: vale o informado", "", "2010-01-01", true},
		{"bureau em formato inválido: vale o informado", "01/01/1980", "2010-01-01", true},
		{"bureau com data impossível: vale o informado", "1980-02-30", "2010-01-01", true},
		{"bureau no futuro: vale o informado", "2030-01-01", "2010-01-01", true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			b := bureauFixo{bureau.Resultado{Score: 920, Situacao: bureau.Regular, Nascimento: c.doBureau}}
			r := req(bureau.CPFScoreAlto, 100_000)
			r.Dados.Nascimento = c.informado
			av := servicoTeste(b, contadorFake{}).avaliar(context.Background(), r, fonteFixa(modelo))
			if menor := motivoFinal(av.reg) == "MENOR_IDADE"; menor != c.menor || (!c.menor && av.reg.Status != "aprovado") {
				t.Fatalf("status %s motivos %v; quer menor=%v", av.reg.Status, av.reg.Motivos, c.menor)
			}
		})
	}
}

// Teste fraco 9: a renda informada prevalece sobre a do bureau; sem renda informada, vale a do bureau.
// Faixa A: limite = min(3.000.000, renda×8). Pedido de 1.000.000.
func TestRendaInformadaPrevalece(t *testing.T) {
	modelo := compilar(t, politica.ModeloEmprestimoPF())
	casos := []struct {
		nome                string
		informada, doBureau *int64
		status              string
		valor               int64
	}{
		{"informada baixa, bureau alta: contraproposta pela informada", ptr(int64(100_000)), ptr(int64(1_200_000)), "contraproposta", 800_000},
		{"informada alta, bureau baixa: aprova pela informada", ptr(int64(1_200_000)), ptr(int64(100_000)), "aprovado", 1_000_000},
		{"sem informada: vale o bureau", nil, ptr(int64(100_000)), "contraproposta", 800_000},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			b := bureauFixo{bureau.Resultado{Score: 920, Situacao: bureau.Regular, Renda: c.doBureau}}
			r := req(bureau.CPFScoreAlto, 1_000_000)
			r.Dados.Renda = c.informada
			av := servicoTeste(b, contadorFake{}).avaliar(context.Background(), r, fonteFixa(modelo))
			if av.reg.Status != c.status || av.reg.Oferta == nil || av.reg.Oferta.Valor != c.valor {
				t.Fatalf("status %s oferta %+v; quer %s %d", av.reg.Status, av.reg.Oferta, c.status, c.valor)
			}
		})
	}
}

// Teste fraco 10: o antifraude roda dentro do mesmo tempo máximo do bureau; contador travado nega no prazo.
func TestAntifraudeDentroDoTempoMaximo(t *testing.T) {
	s := servicoTeste(bureau.NovoSimulado(0), contadorLento{})
	inicio := time.Now()
	av := s.avaliar(context.Background(), req(bureau.CPFScoreAlto, 100_000), fonteFixa(compilar(t, politica.ModeloEmprestimoPF())))
	if decorrido := time.Since(inicio); decorrido > 5*s.timeout {
		t.Fatalf("demorou %v; o timeout era %v", decorrido, s.timeout)
	}
	if av.reg.Status != StatusNegado || motivoFinal(av.reg) != MotivoAntifraudeIndisponivel {
		t.Fatalf("decisão = %s %v", av.reg.Status, av.reg.Motivos)
	}
}

// Achado 12: score fora de 0..1000 é resposta corrompida do bureau: nega como indisponível (fail-closed).
func TestScoreForaDaFaixa(t *testing.T) {
	modelo := compilar(t, politica.ModeloEmprestimoPF())
	casos := []struct {
		score  int
		status string
	}{{-1, StatusNegado}, {1001, StatusNegado}, {1 << 40, StatusNegado}, {0, StatusNegado}, {1000, "aprovado"}}
	for _, c := range casos {
		b := bureauFixo{bureau.Resultado{Score: c.score, Situacao: bureau.Regular}}
		av := servicoTeste(b, contadorFake{}).avaliar(context.Background(), req(bureau.CPFScoreAlto, 100_000), fonteFixa(modelo))
		indisponivel := motivoFinal(av.reg) == MotivoBureauIndisponivel
		foraDaFaixa := c.score < 0 || c.score > 1000
		if av.reg.Status != c.status || indisponivel != foraDaFaixa || (av.reg.Score != nil) == foraDaFaixa {
			t.Errorf("score %d: status %s score %v motivos %v", c.score, av.reg.Status, av.reg.Score, av.reg.Motivos)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestCNPJInexistenteViraDocumentoInvalido(t *testing.T) {
	s := servicoTeste(bureauFixo{bureau.Resultado{Situacao: bureau.Inexistente}}, contadorFake{})
	av := s.avaliar(context.Background(), req("00000000000191", 100_000), fonteFixa(compilar(t, politica.ModeloEmprestimoPF())))
	if av.reg.Status != StatusDocumentoInvalido {
		t.Fatalf("status = %q, quer %q", av.reg.Status, StatusDocumentoInvalido)
	}
}

// serproFalso sobe um Serpro de mentira que responde `status` e conta as chamadas.
func serproFalso(t *testing.T, status int, corpo string, chamadas *int) bureau.Bureau {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*chamadas++
		w.WriteHeader(status)
		_, _ = w.Write([]byte(corpo))
	}))
	t.Cleanup(s.Close)
	return bureau.NovoComposto(serpro.Novo(s.URL, "k", true), nil, bureau.NovoSimulado(0))
}

// Serpro ligado: 422 (menor de 18) alimenta a idade e o corte MENOR_IDADE pega; 451 (menor de 16) é
// documento_invalido; sem nascimento informado o Serpro não é chamado e o cadastro fica nao_verificado.
func TestSerproNaOrquestracao(t *testing.T) {
	modelo := compilar(t, politica.ModeloEmprestimoPF())
	casos := []struct {
		nome, nasc, corpo string
		status            int
		chamadas          int
		decisao, motivo   string
		cadastro          string
	}{
		// o adaptador confere a idade no relógio real: nascimento de 10 anos atrás não envelhece o teste
		{"422 menor de 18", time.Now().AddDate(-10, 0, 0).Format(time.DateOnly), `{}`, 422, 1, StatusNegado, "MENOR_IDADE", bureau.FonteSerproTrial},
		{"451 menor de 16", "2021-07-16", `{}`, 451, 1, StatusDocumentoInvalido, MotivoDocumentoInvalido, ""},
		{"sem nascimento", "", `{}`, 200, 0, "aprovado", "", bureau.FonteNaoVerificado},
		{"regular", "1982-03-08", `{"situacao":{"codigo":"0"},"nome":"BEATRIZ","nascimento":"08031982"}`, 200, 1, "aprovado", "", bureau.FonteSerproTrial},
		{"cota estourada", "1982-03-08", `{"code":"900807"}`, 429, 1, StatusNegado, MotivoBureauIndisponivel, ""},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			n := 0
			r := req(bureau.CPFScoreAlto, 100_000)
			r.Dados.Nascimento = c.nasc
			av := servicoTeste(serproFalso(t, c.status, c.corpo, &n), contadorFake{}).avaliar(context.Background(), r, fonteFixa(modelo))
			if n != c.chamadas || av.reg.Status != c.decisao || motivoFinal(av.reg) != c.motivo {
				t.Fatalf("chamadas %d, status %s, motivos %v", n, av.reg.Status, av.reg.Motivos)
			}
			cad := ""
			if av.reg.Fontes != nil {
				cad = av.reg.Fontes.Cadastro
			}
			if cad != c.cadastro {
				t.Fatalf("fontes %+v, quer cadastro %q", av.reg.Fontes, c.cadastro)
			}
		})
	}
}

// Transparência: fontes, fatores e cadastro. CPF nunca devolve nome nem nascimento da fonte; CNPJ devolve
// razão social e abertura. Sem resultado do bureau, tudo nulo.
func TestFontesNaResposta(t *testing.T) {
	modelo := compilar(t, politica.ModeloEmprestimoPF())
	decidir := func(b bureau.Bureau, doc string) registro {
		return servicoTeste(b, contadorFake{}).avaliar(context.Background(), req(doc, 100_000), fonteFixa(modelo)).reg
	}

	r := decidir(bureau.NovoSimulado(0), bureau.CPFNomeConhecido)
	simulado := bureau.Fontes{Cadastro: "simulado", Score: "simulado", Restricoes: "simulado"}
	if r.Fontes == nil || *r.Fontes != simulado || r.ScoreFatores != nil || r.Cadastro == nil || *r.Cadastro != (Cadastro{Situacao: "regular"}) {
		t.Fatalf("CPF simulado: fontes %+v fatores %+v cadastro %+v", r.Fontes, r.ScoreFatores, r.Cadastro)
	}
	j, _ := json.Marshal(r.Resposta)
	if strings.Contains(string(j), "JOANA") || strings.Contains(string(j), bureau.NascimentoFixture) {
		t.Fatalf("CPF vazou nome ou nascimento da fonte: %s", j)
	}
	for _, campo := range []string{`"fontes":{`, `"score_fatores":null`, `"cadastro":{"situacao":"regular"}`} {
		if !strings.Contains(string(j), campo) {
			t.Fatalf("faltou %s em %s", campo, j)
		}
	}

	empresa := bureau.Resultado{Nome: "BANCO DO BRASIL SA", Situacao: bureau.Regular, Abertura: "1966-08-01",
		Empresa: &bureau.Empresa{CodigoPorte: bureau.PorteDemais}, Fontes: bureau.Fontes{Cadastro: bureau.FonteBrasilAPI}}
	r = decidir(bureau.NovoComposto(nil, bureauFixo{empresa}, bureau.NovoSimulado(0)), "00000000000191")
	quer := Cadastro{Situacao: "regular", RazaoSocial: "BANCO DO BRASIL SA", Abertura: "1966-08-01"}
	if r.Cadastro == nil || *r.Cadastro != quer || len(r.ScoreFatores) != 7 || r.Fontes.Score != bureau.FonteCadastral {
		t.Fatalf("CNPJ: cadastro %+v fatores %+v fontes %+v", r.Cadastro, r.ScoreFatores, r.Fontes)
	}

	for _, doc := range []string{bureau.CPFFalecido, bureau.CPFIndisponivel, "12345678900"} {
		if r := decidir(bureau.NovoSimulado(0), doc); r.Fontes != nil || r.Cadastro != nil || r.ScoreFatores != nil {
			t.Fatalf("%s: fontes %+v cadastro %+v", doc, r.Fontes, r.Cadastro)
		}
	}
}

// Consulta de limite (sem pedido): pré-aprova com limite só quando o motor decide; qualquer falha nega sem limite.
func TestConsultaDeLimite(t *testing.T) {
	modelo := compilar(t, politica.ModeloEmprestimoPF())
	semPedido := func(doc string) Requisicao {
		return Requisicao{Documento: doc, Finalidade: FinalidadeConcessao}
	}
	casos := []struct {
		nome      string
		req       Requisicao
		status    string
		comLimite bool
	}{
		{"pré-aprovado", semPedido(bureau.CPFScoreAlto), string(politica.PreAprovado), true},
		{"com pedido: aprovado e limite", req(bureau.CPFScoreAlto, 100_000), "aprovado", true},
		{"documento inválido", semPedido("123.456.789-00"), StatusDocumentoInvalido, false},
		{"bureau indisponível", semPedido(bureau.CPFIndisponivel), StatusNegado, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			r := servicoTeste(bureau.NovoSimulado(0), contadorFake{}).avaliar(context.Background(), c.req, fonteFixa(modelo)).reg
			if r.Status != c.status || (r.Limite != nil) != c.comLimite || r.Oferta != nil && c.req.Pedido == nil {
				t.Fatalf("status %s limite %+v oferta %+v; quer %s limite=%v", r.Status, r.Limite, r.Oferta, c.status, c.comLimite)
			}
			if c.comLimite && len(r.Limite.Simulacoes) != 3 {
				t.Fatalf("simulações %+v", r.Limite.Simulacoes)
			}
		})
	}
	if campos := semPedido(bureau.CPFScoreAlto).Validar(); len(campos) != 0 {
		t.Fatalf("sem pedido recusado: %v", campos)
	}
	parcial := semPedido(bureau.CPFScoreAlto)
	parcial.Pedido = &Pedido{Valor: 100_000}
	if campos := parcial.Validar(); len(campos) != 1 || campos["pedido.prazo"] == "" {
		t.Fatalf("pedido parcial: %v", campos)
	}
}
