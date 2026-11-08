package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pquerna/otp/totp"

	"github.com/romeroalmeida/motor-credito/internal/painelauth"
	"github.com/romeroalmeida/motor-credito/internal/plataforma"
	"github.com/romeroalmeida/motor-credito/internal/plataforma/bancoteste"
	"github.com/romeroalmeida/motor-credito/internal/repositorio"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

func TestPermitidaExigeAsDuasTravas(t *testing.T) {
	casos := []struct {
		seed, ambiente string
		quer           bool
	}{
		{"on", plataforma.Desenvolvimento, true},
		{"on", plataforma.Teste, true},
		{"on", plataforma.Producao, false},
		{"", plataforma.Desenvolvimento, false},
		{"ON", plataforma.Desenvolvimento, false},
		{"true", plataforma.Teste, false},
	}
	for _, c := range casos {
		if got := Permitida(c.seed, plataforma.Config{Ambiente: c.ambiente}); got != c.quer {
			t.Errorf("Permitida(%q, %q) = %v, quer %v", c.seed, c.ambiente, got, c.quer)
		}
	}
}

// TestSemearPermiteEntrar prova que a senha e o TOTP da semente funcionam no login real do painel.
func TestSemearPermiteEntrar(t *testing.T) {
	b := bancoteste.Novo(t)
	ctx := context.Background()
	chaveCifra := bytes.Repeat([]byte{3}, 32)
	if _, err := semear(ctx, b.App, chaveCifra); err != nil {
		t.Fatalf("primeira semente: %v", err)
	}
	// TOTP no formato antigo (ilegível com subchave + AAD): a semente seguinte troca por um novo.
	if _, err := b.Dono.Exec(ctx, `UPDATE totp SET segredo_cifrado = '\x00'`); err != nil {
		t.Fatal(err)
	}
	d, err := semear(ctx, b.App, chaveCifra) // de novo: reaproveita tenant e admin
	if err != nil {
		t.Fatalf("segunda semente: %v", err)
	}
	pub, err := repositorio.ParaTenant(b.App, d.tenantID).PoliticaPublicada(ctx)
	if err != nil || pub.Versao != 1 {
		t.Fatalf("política publicada: %+v %v", pub, err)
	}
	if !strings.HasPrefix(d.chaveAPI, "ck_live_") {
		t.Fatalf("chave = %q", d.chaveAPI)
	}

	tenants := tenant.NovoServico(b.App)
	r := chi.NewRouter()
	painelauth.Novo(b.App, tenants, painelauth.Opcoes{ChaveCifra: chaveCifra, CookieInseguro: true}).Rotas(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/painel/auth/entrar",
		strings.NewReader(`{"email":"`+emailAdmin+`","senha":"`+d.senha+`"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("entrar: %d %s", w.Code, w.Body.String())
	}
	codigo, err := totp.GenerateCode(d.segredoTOTP, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/painel/auth/2fa", strings.NewReader(`{"codigo":"`+codigo+`"}`))
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
		if c.Name == painelauth.CookieCSRF {
			req.Header.Set(painelauth.CabecalhoCSRF, c.Value)
		}
	}
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusOK || !strings.Contains(w2.Body.String(), `"papel":"admin"`) {
		t.Fatalf("2fa: %d %s", w2.Code, w2.Body.String())
	}
}
