package main

import (
	"strings"
	"testing"

	"github.com/romeroalmeida/motor-credito/internal/plataforma/bancoteste"
)

func TestDiasRetencao(t *testing.T) {
	casos := []struct {
		v    string
		quer int
		erro bool
	}{
		{"", 1825, false}, {" 30 ", 30, false}, {"1", 1, false},
		{"0", 0, true}, {"-5", 0, true}, {"5 anos", 0, true}, {"1e3", 0, true},
	}
	for _, c := range casos {
		d, err := diasRetencao(c.v)
		if d != c.quer || (err != nil) != c.erro {
			t.Errorf("diasRetencao(%q) = %d, %v; quer %d, erro=%v", c.v, d, err, c.quer, c.erro)
		}
	}
}

// O job confere o papel do banco antes de apagar: com o dono (sem RLS) ele se recusa; com motor_app ele roda.
func TestRodarConfereOPapel(t *testing.T) {
	b := bancoteste.Novo(t)
	t.Setenv("RETENCAO_DIAS", "")
	t.Setenv("DATABASE_URL", b.URLDono)
	if err := rodar(); err == nil || !strings.Contains(err.Error(), "escapa da RLS") {
		t.Fatalf("rodar com o dono = %v; quer recusa por escapar da RLS", err)
	}
	t.Setenv("DATABASE_URL", b.URLApp)
	if err := rodar(); err != nil {
		t.Fatalf("rodar com motor_app = %v; quer nil", err)
	}
}
