package bureau

import (
	"context"
	"errors"
	"fmt"

	"github.com/romeroalmeida/motor-credito/internal/documento"
)

type composto struct{ cnpj, base Bureau }

// NovoComposto usa `cnpj` (ex.: BrasilAPI) para o cadastro do CNPJ — Nome, Situacao, Abertura — e `base`
// para o resto (score, restrições, protestos, dívida, renda). CPF vai só para `base`.
// Falha do `cnpj` derruba a consulta: nunca cai em silêncio para o cadastro do `base`.
func NovoComposto(cnpj, base Bureau) Bureau { return composto{cnpj: cnpj, base: base} }

func (c composto) Consultar(ctx context.Context, doc string) (Resultado, error) {
	norm, tipo, err := documento.Normalizar(doc)
	if err != nil {
		return Resultado{}, fmt.Errorf("bureau composto: %w", err)
	}
	if tipo != documento.CNPJ {
		return c.base.Consultar(ctx, norm)
	}
	cad, err := c.cnpj.Consultar(ctx, norm)
	if err != nil && !errors.Is(err, ErrIndisponivel) {
		err = indisponivel(err)
	}
	if err != nil {
		return Resultado{}, err
	}
	if cad.Situacao == Inexistente {
		return cad, nil // sem empresa não há o que pontuar; a orquestração recusa como documento_invalido
	}
	r, err := c.base.Consultar(ctx, norm)
	if err != nil {
		return Resultado{}, err
	}
	r.Nome, r.Situacao, r.Abertura = cad.Nome, cad.Situacao, cad.Abertura
	return r, nil
}
