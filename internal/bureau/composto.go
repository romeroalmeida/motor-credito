package bureau

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/documento"
)

type composto struct {
	cpf, cnpj, base Bureau // cpf/cnpj nil = o base responde tudo daquele tipo
	agora           func() time.Time
}

// NovoComposto usa `cpf` (ex.: Serpro) e `cnpj` (ex.: BrasilAPI) para o cadastro — Nome, Nascimento, Situacao,
// Abertura — e `base` para o resto (restrições, protestos, dívida, renda). Fonte nil = o base responde aquele tipo.
// O score do CNPJ passa a ser o cadastral quando a fonte de CNPJ traz Empresa; o do CPF continua do base.
// Falha de uma fonte real derruba a consulta: nunca cai em silêncio para o cadastro do `base`.
// CPF com fonte real e sem nascimento informado: a fonte não é chamada e o cadastro fica nao_verificado (sem
// situação, nome e nascimento — o do base não vale como se fosse real).
func NovoComposto(cpf, cnpj, base Bureau) Bureau {
	return composto{cpf: cpf, cnpj: cnpj, base: base, agora: time.Now}
}

func (c composto) Consultar(ctx context.Context, doc string) (Resultado, error) {
	return c.ConsultarComNascimento(ctx, doc, "")
}

func (c composto) ConsultarComNascimento(ctx context.Context, doc, nascimento string) (Resultado, error) {
	norm, tipo, err := documento.Normalizar(doc)
	if err != nil {
		return Resultado{}, fmt.Errorf("bureau composto: %w", err)
	}
	fonte := c.cnpj
	if tipo == documento.CPF {
		fonte = c.cpf
	}
	if fonte == nil {
		return c.base.Consultar(ctx, norm)
	}
	if tipo == documento.CPF && nascimento == "" {
		r, err := c.base.Consultar(ctx, norm)
		if err != nil {
			return Resultado{}, err
		}
		r.Nome, r.Nascimento, r.Situacao = "", "", ""
		r.Fontes.Cadastro = FonteNaoVerificado
		return r, nil
	}
	cad, err := Consultar(ctx, fonte, norm, nascimento)
	if err != nil && !errors.Is(err, ErrIndisponivel) {
		err = indisponivel(err)
	}
	if err != nil {
		return Resultado{}, err
	}
	if cad.Situacao == Inexistente || cad.Situacao == Falecido {
		return cad, nil // não há o que pontuar; a orquestração recusa como documento_invalido
	}
	r, err := c.base.Consultar(ctx, norm)
	if err != nil {
		return Resultado{}, err
	}
	r.Nome, r.Nascimento, r.Situacao, r.Abertura, r.Empresa = cad.Nome, cad.Nascimento, cad.Situacao, cad.Abertura, cad.Empresa
	r.Fontes.Cadastro = cad.Fontes.Cadastro
	if cad.Empresa != nil {
		r.Score, r.ScoreFatores = ScoreCadastral(cad.Situacao, cad.Abertura, *cad.Empresa, c.agora())
		r.Fontes.Score = FonteCadastral
	}
	return r, nil
}

// Consultar repassa o nascimento a quem sabe usá-lo (ComNascimento); os outros recebem só o documento.
func Consultar(ctx context.Context, b Bureau, doc, nascimento string) (Resultado, error) {
	if bn, ok := b.(ComNascimento); ok {
		return bn.ConsultarComNascimento(ctx, doc, nascimento)
	}
	return b.Consultar(ctx, doc)
}
