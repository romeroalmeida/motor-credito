package bureau

import (
	"context"
	"errors"
	"testing"
)

// cadastroFake responde fixo e conta as chamadas.
type cadastroFake struct {
	r       Resultado
	err     error
	chamado *int
}

func (f cadastroFake) Consultar(context.Context, string) (Resultado, error) {
	*f.chamado++
	return f.r, f.err
}

const cnpjNumerico = "00000000000191"

func TestComposto(t *testing.T) {
	cad := Resultado{Nome: "BANCO DO BRASIL SA", Situacao: Inativo, Abertura: "1966-08-01", Score: 1, Renda: renda(1)}
	base := NovoSimulado(0)

	t.Run("CNPJ mistura cadastro real com score do base", func(t *testing.T) {
		n := 0
		r, err := NovoComposto(cadastroFake{r: cad, chamado: &n}, base).Consultar(context.Background(), "00.000.000/0001-91")
		if err != nil {
			t.Fatal(err)
		}
		esperado, _ := base.Consultar(context.Background(), cnpjNumerico)
		esperado.Nome, esperado.Situacao, esperado.Abertura = cad.Nome, cad.Situacao, cad.Abertura
		if n != 1 || !iguais(r, esperado) {
			t.Fatalf("chamadas %d, resultado %+v, quer %+v", n, r, esperado)
		}
	})

	t.Run("CPF ignora o real", func(t *testing.T) {
		n := 0
		r, err := NovoComposto(cadastroFake{r: cad, chamado: &n}, base).Consultar(context.Background(), CPFNomeConhecido)
		if err != nil || n != 0 || r.Nome != NomeFixture || r.Abertura != "" {
			t.Fatalf("chamadas %d, resultado %+v, erro %v", n, r, err)
		}
	})

	t.Run("falha do real propaga como indisponível", func(t *testing.T) {
		for _, causa := range []error{ErrIndisponivel, errors.New("qualquer")} {
			n := 0
			r, err := NovoComposto(cadastroFake{err: causa, chamado: &n}, base).Consultar(context.Background(), cnpjNumerico)
			if !errors.Is(err, ErrIndisponivel) || r != (Resultado{}) {
				t.Fatalf("causa %v: resultado %+v, erro %v", causa, r, err)
			}
		}
	})

	t.Run("CNPJ inexistente não consulta o base", func(t *testing.T) {
		n, nBase := 0, 0
		r, err := NovoComposto(cadastroFake{r: Resultado{Situacao: Inexistente}, chamado: &n}, cadastroFake{chamado: &nBase}).Consultar(context.Background(), cnpjNumerico)
		if err != nil || nBase != 0 || r != (Resultado{Situacao: Inexistente}) {
			t.Fatalf("resultado %+v, erro %v, chamadas ao base %d", r, err, nBase)
		}
	})

	t.Run("falha do base propaga", func(t *testing.T) {
		n := 0
		_, err := NovoComposto(cadastroFake{r: cad, chamado: &n}, cadastroFake{err: ErrIndisponivel, chamado: new(int)}).Consultar(context.Background(), cnpjNumerico)
		if !errors.Is(err, ErrIndisponivel) {
			t.Fatalf("erro %v", err)
		}
	})

	t.Run("documento inválido não chama ninguém", func(t *testing.T) {
		n, nBase := 0, 0
		_, err := NovoComposto(cadastroFake{chamado: &n}, cadastroFake{chamado: &nBase}).Consultar(context.Background(), "12345678900")
		if err == nil || n+nBase != 0 {
			t.Fatalf("erro %v, chamadas %d/%d", err, n, nBase)
		}
	})
}
