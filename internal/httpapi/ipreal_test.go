package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/bureau"
	"github.com/romeroalmeida/motor-credito/internal/consulta"
	"github.com/romeroalmeida/motor-credito/internal/httpapi"
	"github.com/romeroalmeida/motor-credito/internal/painelauth"
	"github.com/romeroalmeida/motor-credito/internal/plataforma/bancoteste"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

var proxyTeste = []netip.Prefix{netip.MustParsePrefix("10.9.0.0/24")}

func TestIPReal(t *testing.T) {
	casos := []struct {
		nome, remote string
		xff          []string
		quer         string
	}{
		{"conexão direta ignora XFF", "203.0.113.5:4000", []string{"198.51.100.1"}, "203.0.113.5:4000"},
		{"proxy confiável: cliente à direita", "10.9.0.5:4000", []string{"198.51.100.1"}, "198.51.100.1:0"},
		{"XFF forjado à esquerda é ignorado", "10.9.0.5:4000", []string{"1.2.3.4, 198.51.100.1"}, "198.51.100.1:0"},
		{"pula proxies confiáveis em cadeia", "10.9.0.5:4000", []string{"198.51.100.1, 10.9.0.7"}, "198.51.100.1:0"},
		{"vários cabeçalhos contam como um só", "10.9.0.5:4000", []string{"1.2.3.4", "198.51.100.1"}, "198.51.100.1:0"},
		{"IPv6", "10.9.0.5:4000", []string{"2001:db8::9"}, "[2001:db8::9]:0"},
		{"proxy sem XFF fica com o endereço da conexão", "10.9.0.5:4000", nil, "10.9.0.5:4000"},
		{"XFF ilegível fica com o endereço da conexão", "10.9.0.5:4000", []string{"198.51.100.1, lixo"}, "10.9.0.5:4000"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			var visto string
			h := httpapi.IPReal(proxyTeste)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { visto = r.RemoteAddr }))
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = c.remote
			for _, v := range c.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			h.ServeHTTP(httptest.NewRecorder(), r)
			if visto != c.quer {
				t.Fatalf("RemoteAddr = %q; quer %q", visto, c.quer)
			}
		})
	}
	t.Run("lista vazia não confia em ninguém", func(t *testing.T) {
		var visto string
		h := httpapi.IPReal(nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { visto = r.RemoteAddr }))
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.9.0.5:4000"
		r.Header.Set("X-Forwarded-For", "198.51.100.1")
		h.ServeHTTP(httptest.NewRecorder(), r)
		if visto != "10.9.0.5:4000" {
			t.Fatalf("RemoteAddr = %q", visto)
		}
	})
}

// O router aplica o IP real antes da lista de IPs da chave de API (e dos limitadores do painel).
func TestRouterUsaIPReal(t *testing.T) {
	b := bancoteste.Novo(t)
	ctx := context.Background()
	if _, err := b.Dono.Exec(ctx, `INSERT INTO tenants (id, nome, ips_permitidos) VALUES ('ten_ip', 'ip', '{200.150.10.0/24}')`); err != nil {
		t.Fatal(err)
	}
	tenants := tenant.NovoServico(b.App, chaveCifra)
	chave, err := tenants.CriarChave(ctx, "ten_ip")
	if err != nil {
		t.Fatal(err)
	}
	h := httpapi.Novo(httpapi.Deps{
		Pool: b.App, Tenants: tenants, ProxiesConfiaveis: proxyTeste,
		Auth:      painelauth.Novo(b.App, tenants, painelauth.Opcoes{ChaveCifra: chaveCifra, CookieInseguro: true}),
		Consultas: consulta.Novo(b.App, bureau.NovoSimulado(0), chaveHMAC, chaveCifra),
	})
	chamar := func(remote, xff string) int {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		r := httptest.NewRequest(http.MethodPost, "/v1/consultas", strings.NewReader("x"))
		r.RemoteAddr = remote
		r.Header.Set("X-Forwarded-For", xff)
		r.Header.Set("Authorization", "Bearer "+chave.Chave)
		r.Header.Set("X-Timestamp", ts)
		r.Header.Set("X-Assinatura", tenant.Assinar(chave.Segredo, ts, "", []byte("x")))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	// 400 = passou da autenticação (corpo "x" é inválido); 401 = IP fora da lista.
	if got := chamar("10.9.0.5:4000", "200.150.10.7"); got != http.StatusBadRequest {
		t.Fatalf("cliente permitido atrás do proxy: %d; quer 400", got)
	}
	if got := chamar("10.9.1.5:4000", "200.150.10.7"); got != http.StatusUnauthorized {
		t.Fatalf("XFF vindo de quem não é proxy: %d; quer 401", got)
	}
	if got := chamar("10.9.0.5:4000", "200.150.10.7, 8.8.8.8"); got != http.StatusUnauthorized {
		t.Fatalf("XFF forjado antes do cliente real: %d; quer 401", got)
	}
}
