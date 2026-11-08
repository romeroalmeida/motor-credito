package main

import (
	"strings"
	"testing"

	"github.com/romeroalmeida/motor-credito/internal/plataforma/bancoteste"
)

// O boot (rodar) confere o papel do banco antes de servir: com o dono ele se recusa; com motor_app passa da
// checagem e só para no listen, que recebe uma porta inválida de propósito para o teste não ficar no ar.
func TestRodarConfereOPapel(t *testing.T) {
	b := bancoteste.Novo(t)
	chave := strings.Repeat("ab", 32)
	for k, v := range map[string]string{
		"DATABASE_URL_MIGRACAO": "", "CHAVE_HMAC_DOC": chave, "CHAVE_CIFRA_DOC": chave, "PORTA": "99999",
		"BUREAU_CNPJ": "simulado", "PROXIES_CONFIAVEIS": "", "AMBIENTE": "teste",
	} {
		t.Setenv(k, v)
	}
	t.Setenv("DATABASE_URL", b.URLDono)
	if err := rodar(); err == nil || !strings.Contains(err.Error(), "escapa da RLS") {
		t.Fatalf("boot com o dono = %v; quer recusa por escapar da RLS", err)
	}
	t.Setenv("DATABASE_URL", b.URLApp)
	if err := rodar(); err == nil || strings.Contains(err.Error(), "RLS") || !strings.Contains(err.Error(), "99999") {
		t.Fatalf("boot com motor_app = %v; quer passar da checagem e falhar só no listen da porta 99999", err)
	}
}
