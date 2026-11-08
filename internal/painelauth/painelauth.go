// Package painelauth é a autenticação do painel: senha argon2id + TOTP obrigatório, sessão em cookie
// httpOnly com CSRF double submit, papéis, e as rotas de chaves e equipe.
package painelauth

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romeroalmeida/motor-credito/internal/plataforma"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

// Papel de um usuário dentro de um tenant.
type Papel string

const (
	PapelAdmin    Papel = "admin"    // política, chaves, equipe
	PapelAnalista Papel = "analista" // consultas
	PapelAuditor  Papel = "auditor"  // só auditoria
)

func (p Papel) valido() bool { return p == PapelAdmin || p == PapelAnalista || p == PapelAuditor }

// Sessao é quem está logado. O tenant vem SEMPRE daqui, nunca do cliente.
type Sessao struct {
	UsuarioID, TenantID string
	Papel               Papel
}

// Opcoes do painel. O valor zero é o seguro (cookie Secure ligado).
type Opcoes struct {
	ChaveCifra     []byte // 32 bytes AES-256 para o segredo TOTP (use CHAVE_CIFRA_DOC)
	CookieInseguro bool   // só preencha com CookieInseguroPermitido
	URLPainel      string // base do link de convite, ex.: "https://painel.exemplo.com"; vazio = link relativo
}

// CookieInseguroPermitido tem duas travas: COOKIE_INSEGURO_DEV=on E ambiente explicitamente desenvolvimento
// ou teste. Ambiente vazio ou desconhecido conta como produção.
func CookieInseguroPermitido(cookieInseguroDev, ambiente string) bool {
	return cookieInseguroDev == "on" && (ambiente == plataforma.Desenvolvimento || ambiente == plataforma.Teste)
}

// Handlers atende /painel/auth/*, /painel/chaves* e /painel/equipe.
type Handlers struct {
	pool      *pgxpool.Pool
	tenants   *tenant.Servico
	opcoes    Opcoes
	agora     func() time.Time // relógio do TOTP (trocado nos testes)
	limitador *tenant.Limitador
}

// Novo recebe o pool do usuário da aplicação (membro de motor_app).
func Novo(pool *pgxpool.Pool, tenants *tenant.Servico, opcoes Opcoes) *Handlers {
	return &Handlers{pool: pool, tenants: tenants, opcoes: opcoes, agora: time.Now, limitador: tenant.NovoLimitador()}
}

// Rotas registra as rotas com o caminho completo (/painel/...). Monte no router raiz.
func (h *Handlers) Rotas(r chi.Router) {
	r.Post("/painel/auth/entrar", h.entrar)
	r.Post("/painel/auth/cadastro", h.cadastro)
	r.With(h.carregarSessao(false)).Post("/painel/auth/2fa", h.doisFatores)

	r.Group(func(r chi.Router) {
		r.Use(h.MiddlewareSessao)
		r.Post("/painel/auth/sair", h.sair)
		r.Get("/painel/auth/eu", h.eu)

		r.Group(func(r chi.Router) {
			r.Use(ExigirPapel(PapelAdmin))
			r.Get("/painel/chaves", h.listarChaves)
			r.Post("/painel/chaves", h.criarChave)
			r.Get("/painel/chaves/ips", h.lerIPs)
			r.Put("/painel/chaves/ips", h.definirIPs)
			r.Delete("/painel/chaves/{id}", h.revogarChave)
			r.Post("/painel/chaves/{id}/girar", h.girarChave)
			r.Get("/painel/equipe", h.listarEquipe)
			r.Post("/painel/equipe", h.convidar)
		})
	})
}

// ExigirPapel deixa passar só os papéis listados: sem sessão → 401, papel errado → 403.
// Recurso de outro tenant → 404 é responsabilidade de quem consulta o banco (tenant da sessão + RLS).
func ExigirPapel(papeis ...Papel) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s, ok := SessaoDoContexto(r.Context())
			if !ok {
				escreverErro(w, http.StatusUnauthorized, "nao_autorizado")
				return
			}
			if !slices.Contains(papeis, s.Papel) {
				escreverErro(w, http.StatusForbidden, "proibido")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type chaveContexto struct{}

// sessaoCarregada guarda, além da Sessao pública, o id da linha em sessoes (para sair e girar).
type sessaoCarregada struct {
	Sessao
	id string
}

// SessaoDoContexto devolve a sessão posta pelo MiddlewareSessao.
func SessaoDoContexto(ctx context.Context) (Sessao, bool) {
	s, ok := ctx.Value(chaveContexto{}).(sessaoCarregada)
	return s.Sessao, ok
}

func sessaoInterna(ctx context.Context) (sessaoCarregada, bool) {
	s, ok := ctx.Value(chaveContexto{}).(sessaoCarregada)
	return s, ok
}
