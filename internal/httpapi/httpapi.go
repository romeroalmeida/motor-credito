// Package httpapi monta o router: API pública (/v1), painel (/painel) e /saude.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/netip"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romeroalmeida/motor-credito/internal/consulta"
	"github.com/romeroalmeida/motor-credito/internal/painelauth"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

// MaxCorpo é o limite de corpo de qualquer requisição (contrato: 64 KB).
const MaxCorpo = 64 << 10

// Deps é o que o router precisa; tudo usa o pool da aplicação (motor_app, sujeito à RLS).
type Deps struct {
	Pool      *pgxpool.Pool
	Tenants   *tenant.Servico
	Auth      *painelauth.Handlers
	Consultas *consulta.Servico
	// ProxiesConfiaveis: de quem o X-Forwarded-For vale (PROXIES_CONFIAVEIS). Vazio = de ninguém.
	ProxiesConfiaveis []netip.Prefix
}

type handlers struct{ Deps }

// Novo devolve o http.Handler completo.
func Novo(d Deps) http.Handler {
	h := handlers{d}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer, IPReal(d.ProxiesConfiaveis), cabecalhosSeguranca, limitarCorpo)
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) { escreverErro(w, http.StatusNotFound, "nao_encontrado") })
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		escreverErro(w, http.StatusMethodNotAllowed, "nao_encontrado")
	})

	r.Get("/saude", func(w http.ResponseWriter, _ *http.Request) {
		responder(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.With(d.Tenants.MiddlewareAPI).Post("/v1/consultas", h.criarConsulta)

	d.Auth.Rotas(r)
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.MiddlewareSessao)
		r.Group(func(r chi.Router) {
			r.Use(painelauth.ExigirPapel(painelauth.PapelAdmin, painelauth.PapelAnalista))
			r.Get("/painel/visao-geral", h.visaoGeral)
			r.Get("/painel/consultas", h.listarConsultas)
			r.Post("/painel/consultas/busca", h.buscarConsultas)
			r.Get("/painel/consultas/{id}", h.detalheConsulta)
			r.Get("/painel/politicas", h.estadoPolitica)
			r.Get("/painel/politicas/versoes", h.versoes)
			r.Get("/painel/politicas/versoes/{versao}", h.versao)
			r.Get("/painel/politicas/diferenca", h.diferenca)
		})
		r.Group(func(r chi.Router) {
			r.Use(painelauth.ExigirPapel(painelauth.PapelAdmin, painelauth.PapelAuditor))
			r.Get("/painel/auditoria", h.listarAuditoria)
			r.Get("/painel/auditoria/integridade", h.integridadeAuditoria)
		})
		r.Group(func(r chi.Router) {
			r.Use(painelauth.ExigirPapel(painelauth.PapelAdmin))
			r.Put("/painel/politicas/rascunho", h.salvarRascunho)
			r.Post("/painel/politicas/testar", h.testarPolitica)
			r.Post("/painel/politicas/publicar", h.publicar)
			r.Post("/painel/politicas/versoes/{versao}/reverter", h.reverter)
		})
	})
	return r
}

// cabecalhosSeguranca: a API só devolve JSON; nada pode ser embutido, farejado ou guardado em cache.
func cabecalhosSeguranca(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cab := w.Header()
		cab.Set("X-Content-Type-Options", "nosniff")
		cab.Set("X-Frame-Options", "DENY")
		cab.Set("Referrer-Policy", "no-referrer")
		cab.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		cab.Set("Cross-Origin-Resource-Policy", "same-origin")
		cab.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		cab.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func limitarCorpo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, MaxCorpo)
		next.ServeHTTP(w, r)
	})
}

// erroAPI é o schema Erro do OpenAPI.
type erroAPI struct {
	Erro     string            `json:"erro"`
	Mensagem string            `json:"mensagem,omitempty"`
	Campos   map[string]string `json:"campos,omitempty"`
	Detalhes any               `json:"detalhes,omitempty"`
}

func responder(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func escreverErro(w http.ResponseWriter, status int, codigo string) {
	responder(w, status, erroAPI{Erro: codigo})
}

func erroCampos(w http.ResponseWriter, campos map[string]string) {
	responder(w, http.StatusBadRequest, erroAPI{Erro: "corpo_invalido", Campos: campos})
}

func falhaInterna(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "requisição falhou", "rota", r.URL.Path, "erro", err)
	escreverErro(w, http.StatusInternalServerError, "erro_interno")
}

// lerJSON decodifica o corpo; responde 413 acima do limite e 400 para JSON inválido.
func lerJSON(w http.ResponseWriter, r *http.Request, destino any) bool {
	corpo, err := io.ReadAll(r.Body)
	if err == nil {
		err = json.Unmarshal(corpo, destino)
	}
	if err == nil {
		return true
	}
	var grande *http.MaxBytesError
	if errors.As(err, &grande) {
		escreverErro(w, http.StatusRequestEntityTooLarge, "corpo_grande")
		return false
	}
	escreverErro(w, http.StatusBadRequest, "corpo_invalido")
	return false
}
