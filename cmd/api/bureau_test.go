package main

import (
	"context"
	"testing"

	"github.com/romeroalmeida/motor-credito/internal/bureau"
)

func TestMontarBureau(t *testing.T) {
	for _, v := range []string{"", "simulado", " brasilapi "} {
		if b, err := montarBureau(v); err != nil || b == nil {
			t.Fatalf("BUREAU_CNPJ=%q: %v", v, err)
		}
	}
	for _, v := range []string{"brasilAPI", "serasa", "on"} {
		if _, err := montarBureau(v); err == nil {
			t.Fatalf("BUREAU_CNPJ=%q subiu; quer recusa", v)
		}
	}
	// CPF nunca sai para a rede: com brasilapi, o CPF de fixture continua respondido pelo simulado.
	b, _ := montarBureau("brasilapi")
	r, err := b.Consultar(context.Background(), bureau.CPFScoreAlto)
	if err != nil || r.Score != 920 {
		t.Fatalf("CPF pelo composto: %+v %v", r, err)
	}
}
