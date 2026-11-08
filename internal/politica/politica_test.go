package politica

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/romeroalmeida/motor-credito/internal/dinheiro"
)

func ptr[T any](v T) *T { return &v }

func compilar(t *testing.T, p Politica) *Compilada {
	t.Helper()
	c, err := Compilar(p)
	if err != nil {
		t.Fatalf("Compilar: %v", err)
	}
	return c
}

func avaliar(t *testing.T, c *Compilada, ctx Contexto) Decisao {
	t.Helper()
	d, err := c.Avaliar(ctx)
	if err != nil {
		t.Fatalf("Avaliar: %v", err)
	}
	return d
}

// base é um perfil que passa por todos os cortes do modelo PF.
func base(score int) Contexto {
	return Contexto{Score: score, PedidoValor: 100_000, PedidoPrazo: 12}
}

func codigos(ms []Motivo) string {
	cs := make([]string, len(ms))
	for i, m := range ms {
		cs[i] = m.Codigo
	}
	return strings.Join(cs, ",")
}

func TestModeloPF_LimitesDasFaixas(t *testing.T) {
	c := compilar(t, ModeloEmprestimoPF())
	casos := []struct {
		score  int
		status Status
		faixa  string
		taxa   int
		motivo string
	}{
		{0, Negado, "C", 0, MotivoFaixaNega},
		{300, Negado, "C", 0, MotivoFaixaNega},
		{301, Aprovado, "B", 590, ""},
		{600, Aprovado, "B", 590, ""},
		{601, Aprovado, "A", 290, ""},
		{1000, Aprovado, "A", 290, ""},
		{1001, Negado, "", 0, MotivoSemFaixa},
	}
	for _, tc := range casos {
		t.Run(fmt.Sprint(tc.score), func(t *testing.T) {
			d := avaliar(t, c, base(tc.score))
			if d.Status != tc.status || d.FaixaRisco != tc.faixa || codigos(d.Motivos) != tc.motivo {
				t.Fatalf("got %s/%q/%s; want %s/%q/%s", d.Status, d.FaixaRisco, codigos(d.Motivos), tc.status, tc.faixa, tc.motivo)
			}
			if tc.status == Aprovado && d.Oferta.TaxaBps != tc.taxa {
				t.Fatalf("taxa %d, want %d", d.Oferta.TaxaBps, tc.taxa)
			}
			if tc.status == Negado && d.Oferta != nil {
				t.Fatalf("negado com oferta %+v", d.Oferta)
			}
		})
	}
}

func TestModeloPF_Cortes(t *testing.T) {
	c := compilar(t, ModeloEmprestimoPF())
	casos := []struct {
		nome   string
		mudar  func(*Contexto)
		motivo string
	}{
		{"fraude 69 passa", func(x *Contexto) { x.FraudeRisco = 69 }, ""},
		{"fraude 70 corta", func(x *Contexto) { x.FraudeRisco = 70 }, "FRAUDE_ALTA"},
		{"2 restrições passa", func(x *Contexto) { x.Restricoes = 2 }, ""},
		{"3 restrições corta", func(x *Contexto) { x.Restricoes = 3 }, "RESTRICOES"},
		{"idade 18 passa", func(x *Contexto) { x.Idade = ptr(18) }, ""},
		{"idade 17 corta", func(x *Contexto) { x.Idade = ptr(17) }, "MENOR_IDADE"},
		{"sem idade passa", func(x *Contexto) { x.Idade = nil }, ""},
		// ordem: o primeiro corte que bate é o motivo, mesmo batendo os três
		{"ordem dos cortes", func(x *Contexto) { x.FraudeRisco, x.Restricoes, x.Idade = 90, 5, ptr(10) }, "FRAUDE_ALTA"},
		{"segundo antes do terceiro", func(x *Contexto) { x.Restricoes, x.Idade = 5, ptr(10) }, "RESTRICOES"},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			ctx := base(700)
			tc.mudar(&ctx)
			d := avaliar(t, c, ctx)
			if codigos(d.Motivos) != tc.motivo {
				t.Fatalf("motivos %q, want %q", codigos(d.Motivos), tc.motivo)
			}
			if tc.motivo == "" && d.Status != Aprovado {
				t.Fatalf("status %s, want aprovado", d.Status)
			}
		})
	}
}

func TestNenhumCorteBate_TrilhaCompleta(t *testing.T) {
	d := avaliar(t, compilar(t, ModeloEmprestimoPF()), base(700))
	var cortes []string
	for _, r := range d.Trilha {
		if r.Bloco == blocoCorte {
			if r.Bateu {
				t.Fatalf("corte %s bateu", r.Rotulo)
			}
			cortes = append(cortes, r.Rotulo)
		}
	}
	if got := strings.Join(cortes, ","); got != "FRAUDE_ALTA,RESTRICOES,MENOR_IDADE" {
		t.Fatalf("cortes avaliados %q", got)
	}
	ultimo := d.Trilha[len(d.Trilha)-1]
	if ultimo.Bloco != blocoContraoferta || !ultimo.Bateu {
		t.Fatalf("último passo %+v", ultimo)
	}
}

func TestCorteQueBateParaATrilha(t *testing.T) {
	ctx := base(700)
	ctx.FraudeRisco = 90
	d := avaliar(t, compilar(t, ModeloEmprestimoPF()), ctx)
	if len(d.Trilha) != 1 || !d.Trilha[0].Bateu || d.FaixaRisco != "" {
		t.Fatalf("trilha %+v faixa %q", d.Trilha, d.FaixaRisco)
	}
}

// Limite na faixa A do modelo: min(3.000.000, renda×8).
func TestLimiteEContraproposta(t *testing.T) {
	semContra := ModeloEmprestimoPF()
	semContra.Contraproposta.Ativa = false
	comPrazo := ModeloEmprestimoPF()
	comPrazo.Contraproposta = Contraproposta{Ativa: true, PrazoMaximo: 12, AcrescimoTaxaBps: 50}

	casos := []struct {
		nome   string
		pol    Politica
		renda  *int64
		valor  int64
		prazo  int
		status Status
		oferta *Oferta // Parcela calculada no teste
	}{
		{"renda×8 menor que fixo: pedido no limite aprova", ModeloEmprestimoPF(), ptr[int64](100_000), 800_000, 24, Aprovado, &Oferta{800_000, 24, 290, 0}},
		{"renda×8: um centavo acima vira contraproposta", ModeloEmprestimoPF(), ptr[int64](100_000), 800_001, 24, Contraproposto, &Oferta{800_000, 24, 290, 0}},
		{"contraproposta desligada nega", semContra, ptr[int64](100_000), 800_001, 24, Negado, nil},
		{"renda alta: vale o fixo (min)", ModeloEmprestimoPF(), ptr[int64](10_000_000), 3_000_001, 24, Contraproposto, &Oferta{3_000_000, 24, 290, 0}},
		{"renda ausente: limite fixo aprova no limite", ModeloEmprestimoPF(), nil, 3_000_000, 24, Aprovado, &Oferta{3_000_000, 24, 290, 0}},
		{"renda ausente: acima do fixo", ModeloEmprestimoPF(), nil, 3_000_001, 24, Contraproposto, &Oferta{3_000_000, 24, 290, 0}},
		{"renda zero: limite zero nega", ModeloEmprestimoPF(), ptr[int64](0), 100, 24, Negado, nil},
		{"prazo máximo e acréscimo de taxa", comPrazo, nil, 3_000_001, 24, Contraproposto, &Oferta{3_000_000, 12, 340, 0}},
		{"prazo pedido menor que o máximo fica", comPrazo, nil, 3_000_001, 6, Contraproposto, &Oferta{3_000_000, 6, 340, 0}},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			ctx := Contexto{Score: 700, Renda: tc.renda, PedidoValor: tc.valor, PedidoPrazo: tc.prazo}
			d := avaliar(t, compilar(t, tc.pol), ctx)
			if d.Status != tc.status {
				t.Fatalf("status %s, want %s (%+v)", d.Status, tc.status, d.Motivos)
			}
			if tc.oferta == nil {
				if d.Oferta != nil || codigos(d.Motivos) != MotivoLimiteFaixa {
					t.Fatalf("oferta %+v motivos %q", d.Oferta, codigos(d.Motivos))
				}
				return
			}
			want := *tc.oferta
			want.Parcela, _ = dinheiro.Parcela(want.Valor, want.Prazo, want.TaxaBps)
			if d.Oferta == nil || *d.Oferta != want {
				t.Fatalf("oferta %+v, want %+v", d.Oferta, want)
			}
			if tc.status == Contraproposto && codigos(d.Motivos) != MotivoLimiteFaixa {
				t.Fatalf("motivos %q", codigos(d.Motivos))
			}
		})
	}
}

func TestOrdemDasFaixas_PrimeiraQueBateVale(t *testing.T) {
	// score 250 casa com C (<=300) e com B (<=600); vale C, que nega.
	d := avaliar(t, compilar(t, ModeloEmprestimoPF()), base(250))
	if d.FaixaRisco != "C" || d.Status != Negado {
		t.Fatalf("faixa %q status %s", d.FaixaRisco, d.Status)
	}
}

func TestSemFaixa(t *testing.T) {
	p := Politica{Faixas: []Faixa{{Rotulo: "A", TaxaBps: 100, LimiteFixo: 1000,
		Condicoes: []Condicao{{Campo: "score", Operador: ">=", Valor: 900}}}}}
	d := avaliar(t, compilar(t, p), base(500))
	if d.Status != Negado || codigos(d.Motivos) != MotivoSemFaixa || d.FaixaRisco != "" {
		t.Fatalf("%+v", d)
	}
}

func TestFaixaSemCondicaoCasaComTodos(t *testing.T) {
	p := Politica{Faixas: []Faixa{{Rotulo: "U", TaxaBps: 100, LimiteFixo: 1_000_000}}}
	if d := avaliar(t, compilar(t, p), base(1)); d.Status != Aprovado {
		t.Fatalf("%+v", d)
	}
}

func TestCondicoesVisuais(t *testing.T) {
	faixa := func(cs ...Condicao) Politica {
		return Politica{Faixas: []Faixa{{Rotulo: "X", TaxaBps: 100, LimiteFixo: 1_000_000, Condicoes: cs}}}
	}
	sinais := []any{"ip_datacenter", "email_descartavel"}
	casos := []struct {
		nome string
		pol  Politica
		ctx  Contexto
		bate bool
	}{
		{"sinal presente", faixa(Condicao{"fraude.sinais", "in", sinais}), Contexto{FraudeSinais: []string{"email_descartavel"}}, true},
		{"sinal ausente", faixa(Condicao{"fraude.sinais", "in", sinais}), Contexto{FraudeSinais: []string{"velocidade_alta"}}, false},
		{"sinal único texto", faixa(Condicao{"fraude.sinais", "in", "ip_datacenter"}), Contexto{FraudeSinais: []string{"ip_datacenter"}}, true},
		{"score in lista", faixa(Condicao{"score", "in", []any{float64(10), float64(20)}}), Contexto{Score: 20}, true},
		{"E entre condições", faixa(Condicao{"score", ">", 10}, Condicao{"pedido.prazo", "<=", 12}), Contexto{Score: 11, PedidoPrazo: 13}, false},
		{"renda ausente não casa", faixa(Condicao{"renda", ">=", 0}), Contexto{}, false},
		{"renda presente casa", faixa(Condicao{"renda", ">=", 0}), Contexto{Renda: ptr[int64](0)}, true},
		{"dívida != ", faixa(Condicao{"bureau.divida_total", "!=", 0}), Contexto{DividaTotal: 1}, true},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			tc.ctx.PedidoValor, tc.ctx.PedidoPrazo = 1, max(tc.ctx.PedidoPrazo, 1)
			d := avaliar(t, compilar(t, tc.pol), tc.ctx)
			if (d.Status == Aprovado) != tc.bate {
				t.Fatalf("status %s, bate=%v; trilha %+v", d.Status, tc.bate, d.Trilha)
			}
		})
	}
}

// avancada monta uma política com a expressão do cliente num corte.
func avancada(expressao string) Politica {
	p := ModeloEmprestimoPF()
	p.ExpressaoAvancada = true
	for i := range p.Faixas {
		p.Faixas[i].CEL = "true"
	}
	p.Cortes = []Corte{{Codigo: "X", Texto: "x", CEL: expressao}}
	return p
}

func longa(n int) string {
	partes := make([]string, n)
	for i := range partes {
		partes[i] = fmt.Sprintf("score > %d", i)
	}
	return strings.Join(partes, " && ")
}

func TestCompilar_Recusa(t *testing.T) {
	vazia := ModeloEmprestimoPF()
	vazia.Faixas = nil
	opInvalido := ModeloEmprestimoPF()
	opInvalido.Cortes[0].Condicoes[0].Operador = "=~"
	campoInvalido := ModeloEmprestimoPF()
	campoInvalido.Cortes[0].Condicoes[0].Campo = "salario"
	valorTexto := ModeloEmprestimoPF()
	valorTexto.Cortes[0].Condicoes[0].Valor = "70"
	valorFracao := ModeloEmprestimoPF()
	valorFracao.Cortes[0].Condicoes[0].Valor = 1.5
	sinalComOperador := ModeloEmprestimoPF()
	sinalComOperador.Cortes[0].Condicoes[0] = Condicao{"fraude.sinais", "==", "x"}
	corteVazio := ModeloEmprestimoPF()
	corteVazio.Cortes[0].Condicoes = nil
	semLimite := ModeloEmprestimoPF()
	semLimite.Faixas[1].LimiteFixo = 0
	semRotulo := ModeloEmprestimoPF()
	semRotulo.Faixas[1].Rotulo = ""
	negativo := ModeloEmprestimoPF()
	negativo.Contraproposta.PrazoMaximo = -1
	taxaAcima := ModeloEmprestimoPF()
	taxaAcima.Faixas[1].TaxaBps = TaxaMaximaBps + 1
	acrescimoAcima := ModeloEmprestimoPF()
	acrescimoAcima.Contraproposta.AcrescimoTaxaBps = TaxaMaximaBps + 1
	limiteAcima := ModeloEmprestimoPF()
	limiteAcima.Faixas[1].LimiteFixo = dinheiro.ValorMaximo + 1

	casos := []struct {
		nome   string
		pol    Politica
		regra  string
		motivo string
	}{
		{"sem faixa", vazia, "faixas", "ao menos uma faixa"},
		{"operador fora da lista", opInvalido, "cortes[0]", "operador não permitido"},
		{"campo desconhecido", campoInvalido, "cortes[0]", "campo desconhecido"},
		{"valor texto em campo inteiro", valorTexto, "cortes[0]", "inteiro"},
		{"valor fracionário", valorFracao, "cortes[0]", "inteiro"},
		{"sinais só com in", sinalComOperador, "cortes[0]", "só aceita o operador in"},
		{"corte sem condição", corteVazio, "cortes[0]", "sem condição"},
		{"faixa sem limite", semLimite, "faixas[1]", "limite fixo"},
		{"faixa sem rótulo", semRotulo, "faixas[1]", "sem rótulo"},
		{"contraproposta negativa", negativo, "contraproposta", "negativos"},
		{"limite fixo acima do teto", limiteAcima, "faixas[1]", "limite fixo acima"},
		{"taxa acima do teto", taxaAcima, "faixas[1]", "taxa acima"},
		{"acréscimo acima do teto", acrescimoAcima, "contraproposta", "acréscimo de taxa acima"},
		{"renda sem has", avancada("renda > 100"), "cortes[0]", "renda usada sem has(renda)"},
		{"idade sem has", avancada("idade < 18"), "cortes[0]", "idade usada sem has(idade)"},
		{"has depois do uso", avancada("idade < 18 && has(idade)"), "cortes[0]", "sem has(idade)"},
		{"has com ou", avancada("has(renda) || renda > 1"), "cortes[0]", "sem has(renda)"},
		{"has da outra variável", avancada("has(idade) && renda > 1"), "cortes[0]", "sem has(renda)"},
		{"ternário no ramo falso", avancada("has(renda) ? true : renda > 1"), "cortes[0]", "sem has(renda)"},
		{"has de campo obrigatório", avancada("has(score)"), "cortes[0]", "só aceita renda ou idade"},
		{"mapa oculto", avancada(`"renda" in opcional`), "cortes[0]", "campo desconhecido"},
		{"campo inexistente", avancada("bureau.salario > 1"), "cortes[0]", "undeclared"},
		{"tipo errado", avancada(`score == "alto"`), "cortes[0]", "no matching overload"},
		{"não booleana", avancada("score + 1"), "cortes[0]", "verdadeiro/falso"},
		{"vazia", avancada("  "), "cortes[0]", "expressão vazia"},
		{"custo excessivo", avancada(longa(60)), "cortes[0]", "custo estimado"},
		{"tamanho excessivo", avancada("score > 0 " + strings.Repeat(" ", TamanhoMaximo)), "cortes[0]", "size"},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			_, err := Compilar(tc.pol)
			var ep *ErroPolitica
			if !errors.As(err, &ep) {
				t.Fatalf("erro %v, want *ErroPolitica", err)
			}
			for _, p := range ep.Problemas {
				if p.Regra == tc.regra && strings.Contains(p.Motivo, tc.motivo) {
					return
				}
			}
			t.Fatalf("problemas %+v; want %s ~ %q", ep.Problemas, tc.regra, tc.motivo)
		})
	}
}

// Expressão do cliente: nenhuma função fora da lista, mesmo as da biblioteca padrão do CEL.
func TestExpressaoDoCliente_FuncoesForaDaLista(t *testing.T) {
	proibidas := []string{
		`size(fraude.sinais) > 0`,
		`fraude.sinais.size() > 0`,
		`"x".matches("x")`,
		`string(score) == "1"`,
		`int("1") == 1`,
		`fraude.sinais.exists(s, s == "x")`,
		`fraude.sinais.all(s, s == "x")`,
		`[1, 2].map(x, x * 2) == [2, 4]`,
		`timestamp("2026-01-01T00:00:00Z") > timestamp("2025-01-01T00:00:00Z")`,
		`{"a": 1}["a"] == 1`,
		`"abc".contains("a")`,
		`type(score) == int`,
	}
	for _, expr := range proibidas {
		t.Run(expr, func(t *testing.T) {
			if _, err := Compilar(avancada(expr)); err == nil {
				t.Fatalf("compilou %q", expr)
			}
		})
	}
}

func TestExpressaoDoCliente_Aceitas(t *testing.T) {
	aceitas := []string{
		`has(renda) && pedido.valor > renda * 10`,
		`has(idade) ? idade < 18 : false`,
		`has(renda) && has(idade) && renda < 100 && idade > 60`,
		`"ip_datacenter" in fraude.sinais || fraude.risco >= 70`,
		`!(score in [1, 2, 3]) && bureau.divida_total - 100 > 0`,
		longa(10),
	}
	for _, expr := range aceitas {
		t.Run(expr, func(t *testing.T) {
			c := compilar(t, avancada(expr))
			if _, err := c.Avaliar(base(700)); err != nil {
				t.Fatalf("Avaliar: %v", err)
			}
		})
	}
}

// O limite de custo vale também em execução: sinais além do tamanho assumido na estimativa estouram o custo.
func TestLimiteDeCustoEmExecucao(t *testing.T) {
	c := compilar(t, avancada(`"nao_existe" in fraude.sinais`))
	ctx := base(700)
	ctx.FraudeSinais = make([]string, 10*CustoMaximo)
	if _, err := c.Avaliar(ctx); err == nil || !strings.Contains(err.Error(), "cost") {
		t.Fatalf("erro %v, want estouro de custo", err)
	}
	ctx.FraudeSinais = make([]string, maxSinais)
	if _, err := c.Avaliar(ctx); err != nil {
		t.Fatalf("dentro do limite: %v", err)
	}
}

func TestAvaliar_PedidoInvalido(t *testing.T) {
	c := compilar(t, ModeloEmprestimoPF())
	for _, ctx := range []Contexto{{Score: 700, PedidoPrazo: 12}, {Score: 700, PedidoValor: 100}} {
		if _, err := c.Avaliar(ctx); err == nil {
			t.Fatalf("aceitou %+v", ctx)
		}
	}
}

// Achado 1: o teto de limite_fixo barra na publicação; se ainda assim a parcela não couber em int64
// (política montada sem Compilar), Avaliar devolve erro em vez de oferta com parcela negativa.
func TestLimiteFixoNoTeto(t *testing.T) {
	p := Politica{Faixas: []Faixa{{Rotulo: "A", TaxaBps: 290, LimiteFixo: dinheiro.ValorMaximo}}}
	d := avaliar(t, compilar(t, p), Contexto{PedidoValor: dinheiro.ValorMaximo, PedidoPrazo: 1})
	if d.Status != Aprovado || d.Oferta == nil || d.Oferta.Parcela <= 0 {
		t.Fatalf("no teto: %+v", d)
	}
	c := compilar(t, p)
	c.politica.Faixas[0].LimiteFixo = math.MaxInt64
	if d, err := c.Avaliar(Contexto{PedidoValor: 9_000_000_000_000_000_000, PedidoPrazo: 1}); err == nil {
		t.Fatalf("parcela estourada virou decisão: %+v", d)
	}
}

// Achado 2: taxa e acréscimo no teto exato compilam, e a contraproposta soma sem estourar.
func TestTaxaNoTeto(t *testing.T) {
	p := Politica{Faixas: []Faixa{{Rotulo: "A", TaxaBps: TaxaMaximaBps, LimiteFixo: dinheiro.ValorMaximo}},
		Contraproposta: Contraproposta{Ativa: true, AcrescimoTaxaBps: TaxaMaximaBps}}
	d := avaliar(t, compilar(t, p), Contexto{PedidoValor: dinheiro.ValorMaximo + 1, PedidoPrazo: 1})
	if d.Status != Contraproposto || d.Oferta.TaxaBps != 2*TaxaMaximaBps || d.Oferta.Parcela <= 0 {
		t.Fatalf("no teto: %+v", d.Oferta)
	}
}

func TestErroDivisaoPorZeroNaoViraDecisao(t *testing.T) {
	c := compilar(t, avancada(`score / bureau.restricoes > 1`))
	if _, err := c.Avaliar(base(700)); err == nil {
		t.Fatal("divisão por zero virou decisão")
	}
}

// O JSON que o painel salva (OpenAPI) decodifica, compila e decide; a decisão sai com as tags do contrato.
func TestJSONDoContrato(t *testing.T) {
	entrada := `{
	  "cortes": [{"codigo": "FRAUDE", "texto": "Fraude", "condicoes": [{"campo": "fraude.risco", "operador": ">=", "valor": 70}]}],
	  "faixas": [{"rotulo": "A", "condicoes": [{"campo": "score", "operador": ">", "valor": 500}], "nega": false,
	              "taxa_bps": 290, "limite_fixo": 600000, "multiplo_renda_x10": 0}],
	  "contraproposta": {"ativa": true, "prazo_maximo": 0, "acrescimo_taxa_bps": 0},
	  "expressao_avancada": false
	}`
	var p Politica
	if err := json.Unmarshal([]byte(entrada), &p); err != nil {
		t.Fatal(err)
	}
	d := avaliar(t, compilar(t, p), Contexto{Score: 642, PedidoValor: 1_000_000, PedidoPrazo: 24})
	saida, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, chave := range []string{`"status":"contraproposta"`, `"faixa_risco":"A"`, `"taxa_mensal_bps":290`,
		`"valor":600000`, `"prazo":24`, `"parcela":`, `"codigo":"LIMITE_FAIXA"`, `"bloco":"faixa"`, `"bateu":true`, `"rotulo":"A","expressao":"score `} {
		if !strings.Contains(string(saida), chave) {
			t.Fatalf("falta %s em %s", chave, saida)
		}
	}
	devolta, _ := json.Marshal(p)
	var p2 Politica
	if err := json.Unmarshal(devolta, &p2); err != nil {
		t.Fatal(err)
	}
	if _, err := Compilar(p2); err != nil {
		t.Fatalf("ida e volta: %v", err)
	}
}

func TestErroPolitica_Mensagem(t *testing.T) {
	err := (&ErroPolitica{Problemas: []Problema{{"faixas[1]", "x"}, {"cortes[0]", "y"}}}).Error()
	if err != "política inválida: faixas[1]: x; cortes[0]: y" {
		t.Fatal(err)
	}
}

// Teste fraco 6: renda×múltiplo que estouraria int64 vale o limite fixo, nos dois lados do limite exato.
func TestLimiteDaFaixa_RendaQueEstoura(t *testing.T) {
	f := Faixa{LimiteFixo: 3_000_000, MultiploRenda: 80}
	for _, renda := range []int64{math.MaxInt64 / 80, math.MaxInt64/80 + 1, math.MaxInt64 / 2, math.MaxInt64} {
		if got := limiteDaFaixa(f, &renda); got != f.LimiteFixo {
			t.Errorf("renda %d: limite %d, quer %d", renda, got, f.LimiteFixo)
		}
	}
	d := avaliar(t, compilar(t, ModeloEmprestimoPF()),
		Contexto{Score: 920, Renda: ptr[int64](math.MaxInt64 / 2), PedidoValor: 3_000_000, PedidoPrazo: 12})
	if d.Status != Aprovado || d.Oferta.Valor != 3_000_000 {
		t.Fatalf("renda enorme: %s %+v %v", d.Status, d.Oferta, d.Motivos)
	}
}

// Teste fraco 7: teto do múltiplo de renda no limite exato (1000 compila, 1001 não).
func TestMultiploRenda_Teto(t *testing.T) {
	p := ModeloEmprestimoPF()
	p.Faixas[1].MultiploRenda = maxMultiploRenda
	if _, err := Compilar(p); err != nil {
		t.Fatalf("recusou o teto: %v", err)
	}
	p.Faixas[1].MultiploRenda = maxMultiploRenda + 1
	var ep *ErroPolitica
	if _, err := Compilar(p); !errors.As(err, &ep) || ep.Problemas[0].Regra != "faixas[1]" ||
		!strings.Contains(ep.Problemas[0].Motivo, "múltiplo de renda acima") {
		t.Fatalf("aceitou múltiplo acima do teto: %v", err)
	}
}

// Achados 3 e 4: o valor de uma condição visual precisa sobreviver ao JSON sem mudar. Acima de 2^53−1 o
// float64 do JSON já não representa todo inteiro (2^53+1 vira 2^53) e 2^63 virava MinInt64 em amd64.
func TestValorDaCondicao_InteiroExato(t *testing.T) {
	const seguro = 1<<53 - 1
	politicaCom := func(op string, v any) Politica {
		p := ModeloEmprestimoPF()
		p.Cortes[0].Condicoes[0] = Condicao{Campo: "score", Operador: op, Valor: v}
		return p
	}
	for _, v := range []any{float64(seguro), float64(-seguro), int64(seguro), seguro, float64(0)} {
		if _, err := Compilar(politicaCom(">=", v)); err != nil {
			t.Errorf("recusou %v: %v", v, err)
		}
	}
	recusados := []any{float64(1 << 53), float64(-(1 << 53)), int64(1 << 53), int64(-(1 << 53)), math.Pow(2, 63),
		-math.Pow(2, 63), math.MaxFloat64, int64(math.MaxInt64)}
	for _, v := range recusados {
		if _, err := Compilar(politicaCom(">=", v)); err == nil {
			t.Errorf("aceitou %v", v)
		}
		if _, err := Compilar(politicaCom("in", []any{float64(1), v})); err == nil {
			t.Errorf("aceitou %v na lista", v)
		}
	}
	// Pelo JSON, como o painel envia: 2^53+1 decodifica como 2^53 e precisa ser recusado.
	var c Condicao
	if err := json.Unmarshal([]byte(`{"campo":"score","operador":">=","valor":9007199254740993}`), &c); err != nil {
		t.Fatal(err)
	}
	if _, err := Compilar(politicaCom(c.Operador, c.Valor)); err == nil {
		t.Error("aceitou 2^53+1 vindo do JSON")
	}
}
