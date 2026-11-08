package politica

import (
	"errors"
	"fmt"
	"math"

	"github.com/romeroalmeida/motor-credito/internal/dinheiro"
)

const (
	blocoCorte        = "corte"
	blocoFaixa        = "faixa"
	blocoContraoferta = "contraproposta"
)

// Avaliar aplica a política ao contexto: cortes → faixas → limite → aprovação ou contraproposta.
// Erro (contexto inválido ou expressão que falhou) não é decisão: quem chama nunca deve aprovar nesse caso.
func (c *Compilada) Avaliar(ctx Contexto) (Decisao, error) {
	if ctx.PedidoValor <= 0 || ctx.PedidoPrazo <= 0 {
		return Decisao{}, errors.New("pedido com valor e prazo positivos é obrigatório")
	}
	d := Decisao{Motivos: []Motivo{}, Trilha: []RegraAvaliada{}}
	vars := variaveis(ctx)

	for i, regra := range c.cortes {
		corte := c.politica.Cortes[i]
		bateu, err := avaliarRegra(regra, vars)
		if err != nil {
			return Decisao{}, fmt.Errorf("cortes[%d]: %w", i, err)
		}
		d.Trilha = append(d.Trilha, RegraAvaliada{Bloco: blocoCorte, Rotulo: corte.Codigo, Expressao: regra.expressao, Bateu: bateu})
		if bateu {
			return negar(d, Motivo{Codigo: corte.Codigo, Texto: corte.Texto}), nil
		}
	}

	for i, regra := range c.faixas {
		faixa := c.politica.Faixas[i]
		bateu, err := avaliarRegra(regra, vars)
		if err != nil {
			return Decisao{}, fmt.Errorf("faixas[%d]: %w", i, err)
		}
		d.Trilha = append(d.Trilha, RegraAvaliada{Bloco: blocoFaixa, Rotulo: faixa.Rotulo, Expressao: regra.expressao, Bateu: bateu})
		if bateu {
			d.FaixaRisco = faixa.Rotulo
			return c.ofertar(d, faixa, ctx), nil
		}
	}
	return negar(d, Motivo{Codigo: MotivoSemFaixa, Texto: "Nenhuma faixa de preço atende o perfil"}), nil
}

// ofertar decide a partir da faixa que casou: nega, aprova o pedido ou faz contraproposta no limite.
func (c *Compilada) ofertar(d Decisao, faixa Faixa, ctx Contexto) Decisao {
	if faixa.Nega {
		return negar(d, Motivo{Codigo: MotivoFaixaNega, Texto: "Perfil na faixa " + faixa.Rotulo + " não é atendido"})
	}
	limite := limiteDaFaixa(faixa, ctx.Renda)
	cabe := ctx.PedidoValor <= limite
	d.Trilha = append(d.Trilha, RegraAvaliada{
		Bloco: blocoContraoferta, Rotulo: faixa.Rotulo, Expressao: fmt.Sprintf("pedido.valor <= %d", limite), Bateu: cabe,
	})
	if cabe {
		d.Status = Aprovado
		d.Oferta = novaOferta(ctx.PedidoValor, ctx.PedidoPrazo, faixa.TaxaBps)
		return d
	}

	acimaDoLimite := Motivo{Codigo: MotivoLimiteFaixa, Texto: "Valor pedido acima do limite para o score"}
	cp := c.politica.Contraproposta
	if !cp.Ativa || limite <= 0 {
		return negar(d, acimaDoLimite)
	}
	prazo := ctx.PedidoPrazo
	if cp.PrazoMaximo > 0 {
		prazo = min(prazo, cp.PrazoMaximo)
	}
	d.Status = Contraproposto
	d.Oferta = novaOferta(limite, prazo, faixa.TaxaBps+cp.AcrescimoTaxaBps)
	d.Motivos = append(d.Motivos, acimaDoLimite)
	return d
}

// limiteDaFaixa = min(LimiteFixo, renda×múltiplo/10); sem renda ou múltiplo 0, vale o LimiteFixo.
func limiteDaFaixa(f Faixa, renda *int64) int64 {
	if renda == nil || f.MultiploRenda == 0 {
		return f.LimiteFixo
	}
	if *renda > math.MaxInt64/int64(f.MultiploRenda) {
		return f.LimiteFixo // renda×múltiplo estouraria int64: certamente maior que o limite fixo
	}
	return min(f.LimiteFixo, *renda*int64(f.MultiploRenda)/10)
}

func novaOferta(valor int64, prazo, taxaBps int) *Oferta {
	return &Oferta{Valor: valor, Prazo: prazo, TaxaBps: taxaBps, Parcela: dinheiro.Parcela(valor, prazo, taxaBps)}
}

func negar(d Decisao, m Motivo) Decisao {
	d.Status = Negado
	d.Oferta = nil
	d.Motivos = append(d.Motivos, m)
	return d
}

func avaliarRegra(r regraCompilada, vars map[string]any) (bool, error) {
	saida, _, err := r.programa.Eval(vars)
	if err != nil {
		return false, err
	}
	bateu, ok := saida.Value().(bool)
	if !ok {
		return false, fmt.Errorf("expressão devolveu %T em vez de booleano", saida.Value())
	}
	return bateu, nil
}

// variaveis monta a ativação CEL; renda e idade só entram quando informadas, e o mapa oculto alimenta o has().
func variaveis(ctx Contexto) map[string]any {
	sinais := ctx.FraudeSinais
	if sinais == nil {
		sinais = []string{}
	}
	presentes := map[string]int64{}
	vars := map[string]any{
		"score":               int64(ctx.Score),
		"bureau.restricoes":   int64(ctx.Restricoes),
		"bureau.protestos":    int64(ctx.Protestos),
		"bureau.divida_total": ctx.DividaTotal,
		"pedido.valor":        ctx.PedidoValor,
		"pedido.prazo":        int64(ctx.PedidoPrazo),
		"fraude.risco":        int64(ctx.FraudeRisco),
		campoSinais:           sinais,
		identOpcional:         presentes,
	}
	if ctx.Renda != nil {
		vars["renda"], presentes["renda"] = *ctx.Renda, *ctx.Renda
	}
	if ctx.Idade != nil {
		vars["idade"], presentes["idade"] = int64(*ctx.Idade), int64(*ctx.Idade)
	}
	return vars
}
