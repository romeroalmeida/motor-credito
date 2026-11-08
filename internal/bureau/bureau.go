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
	Renda                 *int64 // centavos/mês
	Abertura              string // AAAA-MM-DD, data de início de atividade do CNPJ
}

// Bureau consulta um documento já normalizado.
type Bureau interface {
	Consultar(ctx context.Context, doc string) (Resultado, error)
}

// ErrIndisponivel indica falha ou timeout do bureau; a decisão vira BUREAU_INDISPONIVEL.
var ErrIndisponivel = errors.New("bureau indisponível")

// indisponivel embrulha a causa mantendo errors.Is com ErrIndisponivel e com a causa (ex.: context.DeadlineExceeded).
func indisponivel(causa error) error {
	return fmt.Errorf("%w: %w", ErrIndisponivel, causa)
}
