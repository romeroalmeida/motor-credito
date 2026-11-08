package politica

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/romeroalmeida/motor-credito/internal/dinheiro"
)

// priceIndependente refaz a Price em float64, por outra fórmula (V·i / (1 − (1+i)^−n)), sem passar por dinheiro.
// Nos valores do teste o float tem folga de sobra para acertar o centavo.
func priceIndependente(valor int64, prazo, taxaBps int) int64 {
	i := float64(taxaBps) / 10_000
	return int64(math.Floor(float64(valor)*i/(1-math.Pow(1+i, -float64(prazo))) + 0.5))
}

func limiteEsperado(valor int64, taxa int, prazos ...int) *Limite {
	l := &Limite{ValorMaximo: valor, TaxaBps: taxa, Simulacoes: []Simulacao{}}
	for _, p := range prazos {
		l.Simulacoes = append(l.Simulacoes, Simulacao{Prazo: p, Parcela: priceIndependente(valor, p, taxa)})
	}
	return l
}

func TestConsultaDeLimite(t *testing.T) {
	// Acréscimo de taxa ligado: a simulação usa a taxa da faixa, não a da contraproposta.
	pol := ModeloEmprestimoPF()
	pol.Contraproposta.AcrescimoTaxaBps = 50
	c := compilar(t, pol)
	casos := []struct {
		nome   string
		ctx    Contexto
		status Status
		motivo string
		limite *Limite
	}{
		{"sem pedido, renda×8 abaixo do fixo", Contexto{Score: 700, Renda: ptr[int64](100_000)},
			PreAprovado, "", limiteEsperado(800_000, 290, 12, 24, 36)},
		{"sem pedido, renda alta: vale o fixo (min)", Contexto{Score: 700, Renda: ptr[int64](10_000_000)},
			PreAprovado, "", limiteEsperado(3_000_000, 290, 12, 24, 36)},
		{"sem pedido, sem renda: fixo da faixa B", Contexto{Score: 450},
			PreAprovado, "", limiteEsperado(1_000_000, 590, 12, 24, 36)},
		{"com pedido aprovado: limite junto", Contexto{Score: 700, Renda: ptr[int64](100_000), PedidoValor: 500_000, PedidoPrazo: 10},
			Aprovado, "", limiteEsperado(800_000, 290, 12, 24, 36)},
		{"contraproposta: limite junto", Contexto{Score: 700, Renda: ptr[int64](100_000), PedidoValor: 900_000, PedidoPrazo: 10},
			Contraproposto, MotivoLimiteFaixa, limiteEsperado(800_000, 290, 12, 24, 36)},
		{"corte bate sem pedido", Contexto{Score: 700, FraudeRisco: 90}, Negado, "FRAUDE_ALTA", nil},
		{"faixa que nega sem pedido", Contexto{Score: 200}, Negado, MotivoFaixaNega, nil},
		{"sem faixa sem pedido", Contexto{Score: 1001}, Negado, MotivoSemFaixa, nil},
		{"renda zero sem pedido", Contexto{Score: 700, Renda: ptr[int64](0)}, Negado, MotivoSemLimite, nil},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			d := avaliar(t, c, tc.ctx)
			if d.Status != tc.status || codigos(d.Motivos) != tc.motivo {
				t.Fatalf("status %s motivos %q; want %s %q", d.Status, codigos(d.Motivos), tc.status, tc.motivo)
			}
			if !reflect.DeepEqual(d.Limite, tc.limite) {
				t.Fatalf("limite %+v; want %+v", d.Limite, tc.limite)
			}
			if semPedido(tc.ctx) && d.Oferta != nil {
				t.Fatalf("sem pedido com oferta %+v", d.Oferta)
			}
		})
	}
}

func TestAvaliar_PedidoParcial(t *testing.T) {
	c := compilar(t, ModeloEmprestimoPF())
	for _, ctx := range []Contexto{{Score: 700, PedidoValor: 100}, {Score: 700, PedidoPrazo: 12}, {Score: 700, PedidoValor: -1, PedidoPrazo: -1}} {
		if d, err := c.Avaliar(ctx); err == nil {
			t.Fatalf("aceitou %+v → %s", ctx, d.Status)
		}
	}
}

// Simulação que estoura int64 é erro da decisão, nunca pré-aprovação com parcela corrompida.
func TestSimulacaoEstourada(t *testing.T) {
	c := compilar(t, Politica{Faixas: []Faixa{{Rotulo: "A", TaxaBps: 290, LimiteFixo: dinheiro.ValorMaximo}},
		Contraproposta: Contraproposta{PrazosOfertados: []int{1, 12}}})
	c.politica.Faixas[0].LimiteFixo = 9_000_000_000_000_000_000 // 9e18 a 2,9%: a de 12x cabe, a de 1x passa de MaxInt64
	d, err := c.Avaliar(Contexto{Score: 700})
	if !errors.Is(err, dinheiro.ErrEstouro) || d.Status != "" || d.Limite != nil {
		t.Fatalf("simulação estourada = %+v, %v; quer erro de estouro e decisão vazia", d, err)
	}
}

func TestPrazosOfertados(t *testing.T) {
	com := func(maximo int, prazos ...int) Politica {
		p := ModeloEmprestimoPF()
		p.Contraproposta.PrazoMaximo, p.Contraproposta.PrazosOfertados = maximo, prazos
		return p
	}
	aceitos := []struct {
		nome string
		pol  Politica
		quer []int
	}{
		{"política antiga sem prazos: padrão", com(0), []int{12, 24, 36}},
		{"padrão cortado no prazo máximo", com(24), []int{12, 24}},
		{"prazo máximo abaixo do padrão: o próprio máximo", com(6), []int{6}},
		{"ordena", com(0, 36, 1, 420), []int{1, 36, 420}},
		{"no prazo máximo exato", com(48, 48, 12), []int{12, 48}},
		{"doze prazos", com(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12), []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}},
	}
	for _, tc := range aceitos {
		t.Run(tc.nome, func(t *testing.T) {
			got := compilar(t, tc.pol).Politica().Contraproposta.PrazosOfertados
			if !reflect.DeepEqual(got, tc.quer) {
				t.Fatalf("prazos %v; want %v", got, tc.quer)
			}
		})
	}
	recusados := []struct {
		nome   string
		pol    Politica
		motivo string
	}{
		{"zero", com(0, 0, 12), "fora de 1 a 420"},
		{"acima do teto", com(0, 12, 421), "fora de 1 a 420"},
		{"repetido", com(0, 24, 12, 24), "repetido"},
		{"treze prazos", com(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13), "de 1 a 12"},
		{"acima do prazo máximo", com(24, 12, 25), "acima do prazo máximo 24"},
	}
	for _, tc := range recusados {
		t.Run(tc.nome, func(t *testing.T) {
			_, err := Compilar(tc.pol)
			var e *ErroPolitica
			if !errors.As(err, &e) || len(e.Problemas) != 1 || e.Problemas[0].Regra != "contraproposta.prazos_ofertados" ||
				!strings.Contains(e.Problemas[0].Motivo, tc.motivo) {
				t.Fatalf("erro %v; quer contraproposta.prazos_ofertados: %s", err, tc.motivo)
			}
		})
	}
	// A simulação usa os prazos normalizados, e o JSON de entrada não é alterado pelo Compilar.
	pol := com(0, 36, 6)
	d := avaliar(t, compilar(t, pol), Contexto{Score: 700})
	if !reflect.DeepEqual(d.Limite, limiteEsperado(3_000_000, 290, 6, 36)) || pol.Contraproposta.PrazosOfertados[0] != 36 {
		t.Fatalf("limite %+v, entrada %v", d.Limite, pol.Contraproposta.PrazosOfertados)
	}
}

// Política antiga relida do banco não ganha "prazos_ofertados": null (o painel trataria null como lista).
func TestPrazosOfertados_JSONSemCampo(t *testing.T) {
	saida, err := json.Marshal(Contraproposta{Ativa: true})
	if err != nil || strings.Contains(string(saida), "prazos_ofertados") {
		t.Fatalf("json %s, %v", saida, err)
	}
}
