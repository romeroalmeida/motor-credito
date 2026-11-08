package politica

import (
	"encoding/json"
	"os"
	"testing"
)

// casoPainel vem de api/cel-do-painel.json: o mesmo arquivo é conferido pelo teste do web
// (web/src/features/politica/utils/campos.test.ts), que prova que o editor gera exatamente estas strings.
type casoPainel struct {
	Condicoes []Condicao `json:"condicoes"`
	CEL       string     `json:"cel"`
}

// TestCELGeradoPeloPainel: a string que o painel gera compila e decide igual às condições visuais.
func TestCELGeradoPeloPainel(t *testing.T) {
	bruto, err := os.ReadFile("../../api/cel-do-painel.json")
	if err != nil {
		t.Fatal(err)
	}
	var casos []casoPainel
	if err := json.Unmarshal(bruto, &casos); err != nil {
		t.Fatal(err)
	}
	contextos := []Contexto{
		{Score: 300, DividaTotal: 600_000},
		{Score: 700, Renda: ptr[int64](400_000), Idade: ptr(17), FraudeSinais: []string{"ip_datacenter"}},
		{Score: 601, Renda: ptr[int64](0), Idade: ptr(18), FraudeSinais: []string{"email_descartavel"}},
	}
	faixa := func(cs []Condicao, cel string) Politica {
		return Politica{ExpressaoAvancada: cel != "", Faixas: []Faixa{{Rotulo: "X", TaxaBps: 100, LimiteFixo: 1_000_000, Condicoes: cs, CEL: cel}}}
	}
	for _, caso := range casos {
		t.Run(caso.CEL, func(t *testing.T) {
			visual, avancada := compilar(t, faixa(caso.Condicoes, "")), compilar(t, faixa(nil, caso.CEL))
			bateuAlguma := false
			for i, ctx := range contextos {
				ctx.PedidoValor, ctx.PedidoPrazo = 1, 1
				dv, da := avaliar(t, visual, ctx), avaliar(t, avancada, ctx)
				if dv.Status != da.Status {
					t.Fatalf("contexto %d: visual %s, CEL do painel %s", i, dv.Status, da.Status)
				}
				bateuAlguma = bateuAlguma || da.Status == Aprovado
			}
			if !bateuAlguma {
				t.Fatal("nenhum contexto casa: o caso não prova nada")
			}
		})
	}
}
