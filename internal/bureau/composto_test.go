package bureau

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

// cadastroFake responde fixo, conta as chamadas e guarda o nascimento recebido.
type cadastroFake struct {
	r          Resultado
	err        error
	chamado    *int
	nascimento *string
}

func (f cadastroFake) Consultar(ctx context.Context, doc string) (Resultado, error) {
	return f.ConsultarComNascimento(ctx, doc, "")
}

func (f cadastroFake) ConsultarComNascimento(_ context.Context, _, nascimento string) (Resultado, error) {
	*f.chamado++
	if f.nascimento != nil {
		*f.nascimento = nascimento
	}
	return f.r, f.err
}

const cnpjNumerico = "00000000000191"

func TestComposto(t *testing.T) {
	cad := Resultado{Nome: "BANCO DO BRASIL SA", Situacao: Inativo, Abertura: "1966-08-01", Score: 1, Renda: renda(1),
		Fontes: Fontes{Cadastro: FonteBrasilAPI}}
	base := NovoSimulado(0)
	vazio := Resultado{}

	t.Run("CNPJ sem Empresa: cadastro real, score do base", func(t *testing.T) {
		n := 0
		r, err := NovoComposto(nil, cadastroFake{r: cad, chamado: &n}, base).Consultar(context.Background(), "00.000.000/0001-91")
		if err != nil {
			t.Fatal(err)
		}
		esperado, _ := base.Consultar(context.Background(), cnpjNumerico)
		esperado.Nome, esperado.Situacao, esperado.Abertura = cad.Nome, cad.Situacao, cad.Abertura
		esperado.Fontes.Cadastro = FonteBrasilAPI
		if n != 1 || !iguais(r, esperado) {
			t.Fatalf("chamadas %d, resultado %+v, quer %+v", n, r, esperado)
		}
	})

	t.Run("CNPJ com Empresa: score cadastral e fatores", func(t *testing.T) {
		emp := &Empresa{CapitalSocial: CapitalFaixa3, CodigoPorte: PorteDemais, Natureza: 2038}
		comEmpresa := cad
		comEmpresa.Situacao, comEmpresa.Empresa = Regular, emp
		c := NovoComposto(nil, cadastroFake{r: comEmpresa, chamado: new(int)}, base).(composto)
		c.agora = func() time.Time { return hojeScore }
		r, err := c.Consultar(context.Background(), cnpjNumerico)
		if err != nil {
			t.Fatal(err)
		}
		score, fatores := ScoreCadastral(Regular, cad.Abertura, *emp, hojeScore)
		simulado, _ := base.Consultar(context.Background(), cnpjNumerico)
		quer := Fontes{Cadastro: FonteBrasilAPI, Score: FonteCadastral, Restricoes: FonteSimulado}
		if r.Score != score || score != 950 || !reflect.DeepEqual(r.ScoreFatores, fatores) || r.Fontes != quer || r.Empresa != emp {
			t.Fatalf("resultado %+v, quer score %d", r, score)
		}
		if r.Restricoes != simulado.Restricoes || r.Protestos != simulado.Protestos || r.DividaTotal != simulado.DividaTotal {
			t.Fatalf("restrições não vieram do base: %+v", r)
		}
	})

	t.Run("CPF sem fonte de CPF vai só para o base", func(t *testing.T) {
		n := 0
		r, err := NovoComposto(nil, cadastroFake{r: cad, chamado: &n}, base).Consultar(context.Background(), CPFNomeConhecido)
		if err != nil || n != 0 || r.Nome != NomeFixture || r.Abertura != "" || r.Fontes.Cadastro != FonteSimulado {
			t.Fatalf("chamadas %d, resultado %+v, erro %v", n, r, err)
		}
	})

	t.Run("CPF com fonte real repassa o nascimento e usa o cadastro dela", func(t *testing.T) {
		n, recebido := 0, ""
		serpro := Resultado{Nome: "RONE PEREIRA", Nascimento: "1970-11-14", Situacao: Irregular, Fontes: Fontes{Cadastro: FonteSerproTrial}}
		r, err := NovoComposto(cadastroFake{r: serpro, chamado: &n, nascimento: &recebido}, nil, base).(ComNascimento).
			ConsultarComNascimento(context.Background(), CPFScoreAlto, "1970-11-14")
		if err != nil || n != 1 || recebido != "1970-11-14" {
			t.Fatalf("chamadas %d, nascimento %q, erro %v", n, recebido, err)
		}
		quer := Fontes{Cadastro: FonteSerproTrial, Score: FonteSimulado, Restricoes: FonteSimulado}
		if r.Situacao != Irregular || r.Nome != serpro.Nome || r.Nascimento != serpro.Nascimento || r.Score != 920 || r.Fontes != quer {
			t.Fatalf("resultado %+v", r)
		}
	})

	t.Run("CPF com fonte real e sem nascimento: não chama e não inventa situação", func(t *testing.T) {
		n := 0
		r, err := NovoComposto(cadastroFake{r: Resultado{Situacao: Regular}, chamado: &n}, nil, base).Consultar(context.Background(), CPFIrregular)
		if err != nil || n != 0 {
			t.Fatalf("chamadas %d, erro %v", n, err)
		}
		if r.Situacao != "" || r.Nome != "" || r.Nascimento != "" || r.Fontes.Cadastro != FonteNaoVerificado || r.Restricoes != 2 {
			t.Fatalf("resultado %+v", r)
		}
	})

	t.Run("falha do real propaga como indisponível", func(t *testing.T) {
		for _, causa := range []error{ErrIndisponivel, errors.New("qualquer")} {
			for _, doc := range []string{cnpjNumerico, CPFScoreAlto} {
				n := 0
				f := cadastroFake{err: causa, chamado: &n}
				r, err := NovoComposto(f, f, base).(ComNascimento).ConsultarComNascimento(context.Background(), doc, "1970-11-14")
				if !errors.Is(err, ErrIndisponivel) || !iguais(r, vazio) {
					t.Fatalf("%s, causa %v: resultado %+v, erro %v", doc, causa, r, err)
				}
			}
		}
	})

	t.Run("inexistente ou falecido não consulta o base", func(t *testing.T) {
		for _, s := range []Situacao{Inexistente, Falecido} {
			n, nBase := 0, 0
			f := cadastroFake{r: Resultado{Situacao: s}, chamado: &n}
			r, err := NovoComposto(f, f, cadastroFake{chamado: &nBase}).(ComNascimento).
				ConsultarComNascimento(context.Background(), CPFScoreAlto, "1970-11-14")
			if err != nil || nBase != 0 || !iguais(r, Resultado{Situacao: s}) {
				t.Fatalf("%s: resultado %+v, erro %v, chamadas ao base %d", s, r, err, nBase)
			}
		}
	})

	t.Run("falha do base propaga", func(t *testing.T) {
		n := 0
		_, err := NovoComposto(nil, cadastroFake{r: cad, chamado: &n}, cadastroFake{err: ErrIndisponivel, chamado: new(int)}).Consultar(context.Background(), cnpjNumerico)
		if !errors.Is(err, ErrIndisponivel) {
			t.Fatalf("erro %v", err)
		}
	})

	t.Run("documento inválido não chama ninguém", func(t *testing.T) {
		n, nBase := 0, 0
		f := cadastroFake{chamado: &n}
		_, err := NovoComposto(f, f, cadastroFake{chamado: &nBase}).Consultar(context.Background(), "12345678900")
		if err == nil || n+nBase != 0 {
			t.Fatalf("erro %v, chamadas %d/%d", err, n, nBase)
		}
	})
}
