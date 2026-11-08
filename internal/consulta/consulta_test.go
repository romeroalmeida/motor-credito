package consulta

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/bureau"
	"github.com/romeroalmeida/motor-credito/internal/politica"
)

type contadorFake struct{ err error }

func (c contadorFake) ConsultasDoc24h(context.Context, []byte) (int, error)       { return 0, c.err }
func (c contadorFake) DocsPorDispositivo24h(context.Context, string) (int, error) { return 0, c.err }

func servicoTeste(b bureau.Bureau, c contadorFake) *Servico {
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
	return Requisicao{Documento: doc, Finalidade: FinalidadeConcessao, Pedido: Pedido{Valor: valor, Prazo: 12}}
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
	r := Requisicao{Documento: "", Finalidade: "outra", Pedido: Pedido{Valor: 0, Prazo: 421},
		Dados: Dados{Nascimento: "07/11/1990", Renda: &renda, Email: "x", IP: "999.1.1.1"}}
	campos := r.Validar()
	for _, c := range []string{"documento", "finalidade", "pedido.valor", "pedido.prazo", "dados.nascimento", "dados.renda", "dados.email", "dados.ip"} {
		if _, ok := campos[c]; !ok {
			t.Errorf("faltou erro em %s (%v)", c, campos)
		}
	}
	if campos := req(bureau.CPFScoreAlto, 1).Validar(); len(campos) != 0 {
		t.Errorf("requisição válida recusada: %v", campos)
	}
}
