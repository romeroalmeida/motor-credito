package painelauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/romeroalmeida/motor-credito/internal/plataforma"
)

const (
	CookieSessao  = "sessao"
	CookieCSRF    = "csrf"
	CabecalhoCSRF = "X-CSRF"

	duracaoPendente = 10 * time.Minute // entre a senha e o TOTP
	duracaoSessao   = 12 * time.Hour   // teto; além dele, 30 min sem uso derrubam a sessão (auth_sessao, migration 0002)
	maxUserAgent    = 300
)

// MiddlewareSessao exige sessão completa (senha + TOTP). Sem sessão → 401. Método que escreve sem
// X-CSRF igual ao cookie csrf e ao token da sessão → 403 csrf_invalido. Papel é relido do banco a cada request.
func (h *Handlers) MiddlewareSessao(next http.Handler) http.Handler {
	return h.carregarSessao(true)(next)
}

// carregarSessao(true) aceita só sessão completa; carregarSessao(false) só a pendente de TOTP.
func (h *Handlers) carregarSessao(completa bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(CookieSessao)
			if err != nil || c.Value == "" {
				escreverErro(w, http.StatusUnauthorized, "nao_autorizado")
				return
			}
			s, csrfHash, doisFatores, err := h.buscarSessao(r.Context(), c.Value)
			if err != nil {
				if !errors.Is(err, pgx.ErrNoRows) {
					slog.ErrorContext(r.Context(), "buscar sessão", "erro", err)
				}
				escreverErro(w, http.StatusUnauthorized, "nao_autorizado")
				return
			}
			if doisFatores != completa {
				escreverErro(w, http.StatusUnauthorized, "nao_autorizado")
				return
			}
			if escreve(r.Method) && !csrfValido(r, csrfHash) {
				escreverErro(w, http.StatusForbidden, "csrf_invalido")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), chaveContexto{}, s)))
		})
	}
}

func (h *Handlers) buscarSessao(ctx context.Context, token string) (sessaoCarregada, []byte, bool, error) {
	var s sessaoCarregada
	var papel string
	var csrfHash []byte
	var doisFatoresEm *time.Time
	err := h.pool.QueryRow(ctx, `SELECT sessao_id, usuario_id, tenant_id, papel, csrf_hash, dois_fatores_em
		FROM auth_sessao($1)`, hashToken(token)).Scan(&s.id, &s.UsuarioID, &s.TenantID, &papel, &csrfHash, &doisFatoresEm)
	if err != nil {
		return sessaoCarregada{}, nil, false, err
	}
	s.Papel = Papel(papel)
	if !s.Papel.valido() {
		return sessaoCarregada{}, nil, false, fmt.Errorf("papel desconhecido %q", papel)
	}
	return s, csrfHash, doisFatoresEm != nil, nil
}

// escreve é tudo que não é leitura: método novo ou desconhecido também exige CSRF (fail-closed).
func escreve(metodo string) bool {
	return metodo != http.MethodGet && metodo != http.MethodHead && metodo != http.MethodOptions
}

// csrfValido: double submit (cabeçalho == cookie) e o token pertence a esta sessão (hash guardado).
func csrfValido(r *http.Request, csrfHash []byte) bool {
	cabecalho := r.Header.Get(CabecalhoCSRF)
	c, err := r.Cookie(CookieCSRF)
	if err != nil || cabecalho == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(cabecalho), []byte(c.Value)) != 1 {
		return false
	}
	return subtle.ConstantTimeCompare(hashToken(cabecalho), csrfHash) == 1
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func tokenAleatorio() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("gerar token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type tokensSessao struct{ sessao, csrf string }

// criarSessao insere a sessão (pendente de TOTP ou completa) dentro de uma transação já em EmTenant.
func criarSessao(ctx context.Context, tx pgx.Tx, r *http.Request, usuarioID, tenantID string, completa bool) (tokensSessao, error) {
	var t tokensSessao
	var err error
	if t.sessao, err = tokenAleatorio(); err != nil {
		return t, err
	}
	if t.csrf, err = tokenAleatorio(); err != nil {
		return t, err
	}
	duracao := duracaoPendente
	if completa {
		duracao = duracaoSessao
	}
	ua := r.UserAgent()
	if len(ua) > maxUserAgent {
		ua = ua[:maxUserAgent]
	}
	_, err = tx.Exec(ctx, `INSERT INTO sessoes (id, token_hash, csrf_hash, usuario_id, tenant_id, dois_fatores_em, expira_em, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5, CASE WHEN $6 THEN now() END, now() + make_interval(secs => $7), $8, $9)`,
		plataforma.NovoID("ses"), hashToken(t.sessao), hashToken(t.csrf), usuarioID, tenantID, completa,
		duracao.Seconds(), ipOuNulo(r), ua)
	if err != nil {
		return t, fmt.Errorf("criar sessão: %w", err)
	}
	return t, nil
}

// ipDoCliente lê o IP de r.RemoteAddr, que httpapi.IPReal já reescreveu quando a conexão veio de proxy confiável.
func ipDoCliente(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}

// ipOuNulo: a coluna inet aceita NULL quando o endereço é ilegível.
func ipOuNulo(r *http.Request) any {
	if ip, ok := ipDoCliente(r); ok {
		return ip
	}
	return nil
}

func (h *Handlers) definirCookies(w http.ResponseWriter, t tokensSessao, completa bool) {
	duracao := duracaoPendente
	if completa {
		duracao = duracaoSessao
	}
	h.cookie(w, CookieSessao, t.sessao, int(duracao.Seconds()), true)
	h.cookie(w, CookieCSRF, t.csrf, int(duracao.Seconds()), false)
}

func (h *Handlers) apagarCookies(w http.ResponseWriter) {
	h.cookie(w, CookieSessao, "", -1, true)
	h.cookie(w, CookieCSRF, "", -1, false)
}

// cookie: o csrf não é httpOnly porque o JS do painel precisa lê-lo para o cabeçalho X-CSRF.
func (h *Handlers) cookie(w http.ResponseWriter, nome, valor string, maxAge int, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{
		Name: nome, Value: valor, Path: "/", MaxAge: maxAge,
		HttpOnly: httpOnly, Secure: !h.opcoes.CookieInseguro, SameSite: http.SameSiteStrictMode,
	})
}
