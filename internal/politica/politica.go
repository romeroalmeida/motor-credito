// Package politica guarda a política de crédito editável pelo cliente e o motor que a avalia.
//
// A política tem três blocos avaliados nesta ordem: cortes (o primeiro que bate nega), faixas (a primeira
// que bate define taxa e limite) e contraproposta (o que fazer quando o pedido passa do limite).
// Toda condição vira uma expressão CEL; a do editor visual é gerada aqui, a do modo avançado vem do cliente.
// As duas passam pela mesma validação em Compilar.
package politica

// Politica é o modelo editável: o JSON que o painel salva.
type Politica struct {
	Cortes            []Corte        `json:"cortes"`
	Faixas            []Faixa        `json:"faixas"`
	Contraproposta    Contraproposta `json:"contraproposta"`
	ExpressaoAvancada bool           `json:"expressao_avancada"` // true = condições vieram escritas em CEL pelo cliente
}

// Condicao é uma linha do editor visual: {"score","<=",100}.
type Condicao struct {
	Campo    string `json:"campo"`
	Operador string `json:"operador"`
	Valor    any    `json:"valor"`
}

// Corte nega quando bate. Condições são ligadas por E; no modo avançado vale o CEL.
type Corte struct {
	Codigo    string     `json:"codigo"`
	Texto     string     `json:"texto"`
	Condicoes []Condicao `json:"condicoes"`
	CEL       string     `json:"cel"`
}

// Faixa define taxa e limite para quem casa com ela. Sem condições, casa com todos.
type Faixa struct {
	Rotulo        string     `json:"rotulo"` // "A","B","C"...
	Condicoes     []Condicao `json:"condicoes"`
	CEL           string     `json:"cel"`
	Nega          bool       `json:"nega"`
	TaxaBps       int        `json:"taxa_bps"`
	LimiteFixo    int64      `json:"limite_fixo"`        // centavos; usado se não houver renda ou multiplicador 0
	MultiploRenda int        `json:"multiplo_renda_x10"` // 30 = 3,0× renda
}

// Contraproposta diz o que oferecer quando o pedido passa do limite da faixa.
type Contraproposta struct {
	Ativa            bool `json:"ativa"`
	PrazoMaximo      int  `json:"prazo_maximo"` // 0 = mantém o prazo pedido
	AcrescimoTaxaBps int  `json:"acrescimo_taxa_bps"`
}

// Contexto é o que as regras enxergam. Renda e Idade nil = não informados.
type Contexto struct {
	Score                 int
	Restricoes, Protestos int
	DividaTotal           int64
	Renda                 *int64
	Idade                 *int
	PedidoValor           int64
	PedidoPrazo           int
	FraudeRisco           int
	FraudeSinais          []string
}

// Status é o resultado da decisão do motor.
type Status string

const (
	Aprovado       Status = "aprovado"
	Negado         Status = "negado"
	Contraproposto Status = "contraproposta"
)

// Motivo explica uma negativa ou contraproposta.
type Motivo struct {
	Codigo string `json:"codigo"`
	Texto  string `json:"texto"`
}

// Oferta é o crédito oferecido. Valores em centavos, taxa em pontos-base ao mês.
type Oferta struct {
	Valor   int64 `json:"valor"`
	Prazo   int   `json:"prazo"`
	TaxaBps int   `json:"taxa_mensal_bps"`
	Parcela int64 `json:"parcela"`
}

// RegraAvaliada é um passo da trilha mostrada no detalhe da consulta.
type RegraAvaliada struct {
	Bloco     string `json:"bloco"`  // corte | faixa | contraproposta
	Rotulo    string `json:"rotulo"` // código do corte ou rótulo da faixa
	Expressao string `json:"expressao"`
	Bateu     bool   `json:"bateu"`
}

// Decisao é a saída do motor.
type Decisao struct {
	Status     Status          `json:"status"`
	FaixaRisco string          `json:"faixa_risco"`
	Oferta     *Oferta         `json:"oferta"`
	Motivos    []Motivo        `json:"motivos"`
	Trilha     []RegraAvaliada `json:"trilha"`
}

// Códigos de motivo produzidos pelo próprio motor (os de corte vêm da política).
const (
	MotivoSemFaixa    = "SEM_FAIXA"
	MotivoLimiteFaixa = "LIMITE_FAIXA"
	MotivoFaixaNega   = "FAIXA_NEGADA"
)

// ModeloEmprestimoPF é a política inicial do desenho §5.
func ModeloEmprestimoPF() Politica {
	return Politica{
		Cortes: []Corte{
			{Codigo: "FRAUDE_ALTA", Texto: "Risco de fraude alto",
				Condicoes: []Condicao{{Campo: "fraude.risco", Operador: ">=", Valor: 70}}},
			{Codigo: "RESTRICOES", Texto: "Mais de duas restrições no bureau",
				Condicoes: []Condicao{{Campo: "bureau.restricoes", Operador: ">", Valor: 2}}},
			{Codigo: "MENOR_IDADE", Texto: "Solicitante menor de idade",
				Condicoes: []Condicao{{Campo: "idade", Operador: "<", Valor: 18}}},
		},
		Faixas: []Faixa{
			{Rotulo: "C", Nega: true,
				Condicoes: []Condicao{{Campo: "score", Operador: "<=", Valor: 300}}},
			{Rotulo: "B", TaxaBps: 590, LimiteFixo: 1_000_000, MultiploRenda: 30,
				Condicoes: []Condicao{{Campo: "score", Operador: "<=", Valor: 600}}},
			{Rotulo: "A", TaxaBps: 290, LimiteFixo: 3_000_000, MultiploRenda: 80,
				Condicoes: []Condicao{{Campo: "score", Operador: "<=", Valor: 1000}}},
		},
		Contraproposta: Contraproposta{Ativa: true},
	}
}
