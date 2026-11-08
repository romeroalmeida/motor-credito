package bureau

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/documento"
)

func TestFixturesTemDigitosValidos(t *testing.T) {
	for _, doc := range []string{CPFFalecido, CPFIrregular, CPFScoreBaixo, CPFScoreAlto, CPFNomeConhecido,
		CPFIndisponivel, CPFLento, CPFCamposAusentes, CNPJInativo, CNPJRegular} {
		if got, _, err := documento.Normalizar(doc); err != nil || got != doc {
			t.Errorf("fixture %s inválida: %v", doc, err)
		}
	}
}

func TestFixtures(t *testing.T) {
	b := NovoSimulado(0)
	casos := []struct {
		doc  string
		quer func(Resultado) bool
	}{
		{CPFFalecido, func(r Resultado) bool { return r.Situacao == Falecido }},
		{CPFIrregular, func(r Resultado) bool { return r.Situacao == Irregular }},
		{CPFScoreBaixo, func(r Resultado) bool { return r.Score < 300 && r.Situacao == Regular }},
		{CPFScoreAlto, func(r Resultado) bool { return r.Score > 900 && r.Renda != nil }},
		{CPFNomeConhecido, func(r Resultado) bool { return r.Nome == NomeFixture && r.Nascimento == NascimentoFixture }},
		{CPFCamposAusentes, func(r Resultado) bool { return r.Nome == "" && r.Nascimento == "" && r.Renda == nil }},
		{CNPJInativo, func(r Resultado) bool { return r.Situacao == Inativo }},
		{"exemplo.00001/54", func(r Resultado) bool { return r.Situacao == Inativo }}, // máscara e caixa
		{CNPJRegular, func(r Resultado) bool { return r.Situacao == Regular && r.Score > 800 }},
	}
	for _, c := range casos {
		r, err := b.Consultar(context.Background(), c.doc)
		if err != nil || !c.quer(r) {
			t.Errorf("%s: resultado %+v, erro %v", c.doc, r, err)
		}
	}
}

func TestIndisponivel(t *testing.T) {
	_, err := NovoSimulado(0).Consultar(context.Background(), CPFIndisponivel)
	if !errors.Is(err, ErrIndisponivel) {
		t.Fatalf("erro = %v, quer ErrIndisponivel", err)
	}
}

func TestDocumentoInvalido(t *testing.T) {
	_, err := NovoSimulado(0).Consultar(context.Background(), "12345678900")
	if !errors.Is(err, documento.ErrInvalido) {
		t.Fatalf("erro = %v, quer documento.ErrInvalido", err)
	}
}

func TestDeterministico(t *testing.T) {
	a, b := NovoSimulado(0), NovoSimulado(0)
	situacoes := map[Situacao]bool{}
	semRenda, semNome := false, false
	for i := 0; i < 200; i++ {
		doc := cpfValido(fmt.Sprintf("1%08d", i*7919))
		r1, err1 := a.Consultar(context.Background(), doc)
		r2, err2 := b.Consultar(context.Background(), doc)
		if err1 != nil || err2 != nil || !iguais(r1, r2) {
			t.Fatalf("%s não é determinístico: %+v %v / %+v %v", doc, r1, err1, r2, err2)
		}
		situacoes[r1.Situacao] = true
		semRenda = semRenda || r1.Renda == nil
		semNome = semNome || r1.Nome == ""
	}
	if len(situacoes) < 3 || !semRenda || !semNome {
		t.Errorf("perfis pouco variados: situações %v, semRenda %v, semNome %v", situacoes, semRenda, semNome)
	}
	// documentos diferentes não podem colapsar no mesmo perfil
	r1, _ := a.Consultar(context.Background(), cpfValido("123456789"))
	r2, _ := a.Consultar(context.Background(), cpfValido("987654321"))
	if iguais(r1, r2) {
		t.Errorf("documentos diferentes deram o mesmo perfil: %+v", r1)
	}
}

func TestRespeitaCtx(t *testing.T) {
	casos := []struct {
		nome   string
		atraso time.Duration
		doc    string
	}{
		{"atraso configurado", time.Hour, CPFScoreAlto},
		{"fixture lenta", 0, CPFLento},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			inicio := time.Now()
			_, err := NovoSimulado(c.atraso).Consultar(ctx, c.doc)
			if !errors.Is(err, ErrIndisponivel) || !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("erro = %v, quer ErrIndisponivel + DeadlineExceeded", err)
			}
			if d := time.Since(inicio); d > time.Second {
				t.Fatalf("demorou %v, não respeitou o ctx", d)
			}
		})
	}
}

func TestAtrasoAplicado(t *testing.T) {
	inicio := time.Now()
	if _, err := NovoSimulado(30*time.Millisecond).Consultar(context.Background(), CPFScoreAlto); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(inicio); d < 30*time.Millisecond {
		t.Fatalf("respondeu em %v, antes do atraso", d)
	}
}

func iguais(a, b Resultado) bool {
	ra, rb := a.Renda, b.Renda
	a.Renda, b.Renda = nil, nil
	return a == b && (ra == nil) == (rb == nil) && (ra == nil || *ra == *rb)
}

// cpfValido completa 9 dígitos com os dois verificadores.
func cpfValido(base string) string {
	for len(base) < 11 {
		soma := 0
		for j := 0; j < len(base); j++ {
			soma += int(base[j]-'0') * (len(base) + 1 - j)
		}
		dv := 0
		if r := soma % 11; r >= 2 {
			dv = 11 - r
		}
		base += string(rune('0' + dv))
	}
	return base
}
