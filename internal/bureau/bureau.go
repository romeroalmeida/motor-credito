// Package bureau define o contrato de consulta a bureau de crédito e um adaptador simulado determinístico.
package bureau

import (
	"context"
	"errors"
	"fmt"
)

// Situacao é a situação cadastral do documento na base do bureau.
type Situacao string

const (
	Regular   Situacao = "regular"
	Irregular Situacao = "irregular"
	Falecido  Situacao = "falecido"
	Inativo   Situacao = "inativo"
	// Inexistente: o documento não consta na base consultada (ex.: CNPJ sem registro). A orquestração trata como documento_invalido.
	Inexistente Situacao = "inexistente"
)

// Resultado é o que o bureau devolve. Campo vazio ("" ou nil) significa que o bureau não tem o dado.
type Resultado struct {
	Score                 int
	Restricoes, Protestos int
	DividaTotal           int64 // centavos
	Nome                  string
	Nascimento            string // AAAA-MM-DD
	Situacao              Situacao
	Renda                 *int64   // centavos/mês
	Abertura              string   // AAAA-MM-DD, data de início de atividade do CNPJ
	Empresa               *Empresa // cadastro público do CNPJ (BrasilAPI); nil quando a fonte não traz
	ScoreFatores          []Fator  // só quando o score é o cadastral; nil no simulado
	Fontes                Fontes   // de onde veio cada parte; cada adaptador preenche a sua
}

// Fontes conta de onde veio cada parte do resultado (bloco `fontes` da resposta).
type Fontes struct {
	Cadastro   string `json:"cadastro"`   // FonteSerpro, FonteSerproTrial, FonteBrasilAPI, FonteSimulado ou FonteNaoVerificado
	Score      string `json:"score"`      // FonteCadastral ou FonteSimulado
	Restricoes string `json:"restricoes"` // sempre FonteSimulado: não há fonte grátis de restrições
}

// Nomes das fontes (enum do api/openapi.yaml).
const (
	FonteSimulado      = "simulado"
	FonteSerpro        = "serpro"
	FonteSerproTrial   = "serpro-trial"
	FonteBrasilAPI     = "brasilapi"
	FonteNaoVerificado = "nao_verificado"
	FonteCadastral     = "cadastral"
)

// Empresa são os dados públicos do CNPJ que alimentam o score cadastral. Sócios: só a contagem, nunca quem são.
type Empresa struct {
	CapitalSocial int64 // centavos
	CodigoPorte   int   // 1 micro, 3 pequeno porte, 5 demais, 0 não informado (Receita)
	Simples, MEI  bool
	Natureza      int // código da natureza jurídica (ex.: 2062 sociedade limitada)
	Socios        int
}

// Fator é uma parcela explicável do score cadastral (ex.: TEMPO_EMPRESA_5A_MAIS +200).
type Fator struct {
	Codigo string `json:"codigo"`
	Pontos int    `json:"pontos"`
}

// Bureau consulta um documento já normalizado.
type Bureau interface {
	Consultar(ctx context.Context, doc string) (Resultado, error)
}

// ComNascimento é implementado pelo bureau que usa o nascimento informado (o Serpro exige para consultar CPF).
// A orquestração prefere esta forma quando existe; nascimento "" = não informado.
type ComNascimento interface {
	ConsultarComNascimento(ctx context.Context, doc, nascimento string) (Resultado, error)
}

// ErrIndisponivel indica falha ou timeout do bureau; a decisão vira BUREAU_INDISPONIVEL.
var ErrIndisponivel = errors.New("bureau indisponível")

// indisponivel embrulha a causa mantendo errors.Is com ErrIndisponivel e com a causa (ex.: context.DeadlineExceeded).
func indisponivel(causa error) error {
	return fmt.Errorf("%w: %w", ErrIndisponivel, causa)
}
