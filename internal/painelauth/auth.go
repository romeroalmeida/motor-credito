package painelauth

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/romeroalmeida/motor-credito/internal/plataforma"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

const (
	limiteEntrar = 10 // tentativas de senha por e-mail+IP por janela
	limite2FA    = 5  // códigos TOTP por usuário por janela
	janelaLimite = 15 * time.Minute

	minSenha, maxSenha = 12, 128
	maxNome            = 120
)

var (
	errCodigoRecusado  = errors.New("código TOTP recusado")
	errConviteInvalido = errors.New("convite inexistente, usado ou vencido")
	errEmailEmUso      = errors.New("e-mail já cadastrado")
)

type respostaEu struct {
	Usuario struct {
		ID    string `json:"id"`
		Nome  string `json:"nome"`
		Email string `json:"email"`
	} `json:"usuario"`
	Tenant struct {
		ID   string `json:"id"`
		Nome string `json:"nome"`
	} `json:"tenant"`
	Papel Papel `json:"papel"`
}

// entrar é o passo da senha. E-mail inexistente, senha errada e usuário sem tenant respondem igual (401).
func (h *Handlers) entrar(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Email string `json:"email"`
		Senha string `json:"senha"`
	}
	if !ler(w, r, &c, false) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(c.Email))
	if email == "" || c.Senha == "" {
		erroCampos(w, map[string]string{"email": "obrigatório", "senha": "obrigatória"})
		return
	}
	if !h.permitir(w, "entrar|"+email+"|"+ipTexto(r), limiteEntrar) {
		return
	}
	usuarioID, tenantID, ok, err := h.conferirCredenciais(r.Context(), email, c.Senha)
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	if !ok {
		escreverErro(w, http.StatusUnauthorized, "nao_autorizado")
		return
	}
	var t tokensSessao
	err = tenant.EmTenant(r.Context(), h.pool, tenantID, func(tx pgx.Tx) error {
		t, err = criarSessao(r.Context(), tx, r, usuarioID, tenantID, false)
		return err
	})
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	h.definirCookies(w, t, false)
	responder(w, http.StatusOK, map[string]string{"proximo": "2fa"})
}

// conferirCredenciais devolve o usuário e o primeiro tenant dele.
// shortcut: usuário em vários tenants entra sempre no mais antigo; troca de tenant fica para quando houver caso real.
func (h *Handlers) conferirCredenciais(ctx context.Context, email, senha string) (usuarioID, tenantID string, ok bool, err error) {
	var hash string
	err = h.pool.QueryRow(ctx, `SELECT id, senha_hash FROM usuarios WHERE lower(email) = $1`, email).Scan(&usuarioID, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		conferirSenha(senha, hashFicticio())
		return "", "", false, nil
	}
	if err != nil || !conferirSenha(senha, hash) {
		return "", "", false, err
	}
	err = h.pool.QueryRow(ctx, `SELECT tenant_id FROM auth_associacoes($1) LIMIT 1`, usuarioID).Scan(&tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	return usuarioID, tenantID, err == nil, err
}

// doisFatores confere o TOTP da sessão pendente e troca por uma sessão completa (cookie novo).
func (h *Handlers) doisFatores(w http.ResponseWriter, r *http.Request) {
	s, _ := sessaoInterna(r.Context())
	var c struct {
		Codigo string `json:"codigo"`
	}
	if !ler(w, r, &c, false) {
		return
	}
	if !h.permitir(w, "2fa|"+s.UsuarioID, limite2FA) {
		return
	}
	var t tokensSessao
	var eu respostaEu
	err := tenant.EmTenant(r.Context(), h.pool, s.TenantID, func(tx pgx.Tx) error {
		if err := h.consumirCodigo(r.Context(), tx, s.UsuarioID, c.Codigo); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE sessoes SET encerrada_em = now() WHERE id = $1 AND tenant_id = $2`,
			s.id, s.TenantID); err != nil {
			return err
		}
		var err error
		if t, err = criarSessao(r.Context(), tx, r, s.UsuarioID, s.TenantID, true); err != nil {
			return err
		}
		eu, err = carregarEu(r.Context(), tx, s.Sessao)
		return err
	})
	if errors.Is(err, errCodigoRecusado) {
		escreverErro(w, http.StatusUnauthorized, "nao_autorizado")
		return
	}
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	h.definirCookies(w, t, true)
	responder(w, http.StatusOK, eu)
}

// consumirCodigo aceita o código uma vez só: o passo precisa ser maior que o último usado (anti-replay).
// Também conclui o cadastro do TOTP (confirmado_em) no primeiro código certo.
func (h *Handlers) consumirCodigo(ctx context.Context, tx pgx.Tx, usuarioID, codigo string) error {
	var cifrado []byte
	err := tx.QueryRow(ctx, `SELECT segredo_cifrado FROM totp WHERE usuario_id = $1`, usuarioID).Scan(&cifrado)
	if errors.Is(err, pgx.ErrNoRows) {
		return errCodigoRecusado
	}
	if err != nil {
		return err
	}
	segredo, err := DecifrarTOTP(h.opcoes.ChaveCifra, usuarioID, cifrado)
	if err != nil {
		return err
	}
	passo, ok := passoDoCodigo(segredo, codigo, h.agora())
	if !ok {
		return errCodigoRecusado
	}
	tag, err := tx.Exec(ctx, `UPDATE totp SET ultimo_passo = $2, confirmado_em = coalesce(confirmado_em, now())
		WHERE usuario_id = $1 AND ultimo_passo < $2`, usuarioID, passo)
	if err == nil && tag.RowsAffected() == 0 {
		return errCodigoRecusado
	}
	return err
}

func carregarEu(ctx context.Context, tx pgx.Tx, s Sessao) (respostaEu, error) {
	var eu respostaEu
	eu.Papel = s.Papel
	err := tx.QueryRow(ctx, `SELECT u.id, u.nome, u.email, t.id, t.nome FROM usuarios u, tenants t
		WHERE u.id = $1 AND t.id = $2`, s.UsuarioID, s.TenantID).
		Scan(&eu.Usuario.ID, &eu.Usuario.Nome, &eu.Usuario.Email, &eu.Tenant.ID, &eu.Tenant.Nome)
	return eu, err
}

func (h *Handlers) eu(w http.ResponseWriter, r *http.Request) {
	s, _ := SessaoDoContexto(r.Context())
	var eu respostaEu
	err := tenant.EmTenant(r.Context(), h.pool, s.TenantID, func(tx pgx.Tx) error {
		var err error
		eu, err = carregarEu(r.Context(), tx, s)
		return err
	})
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusOK, eu)
}

func (h *Handlers) sair(w http.ResponseWriter, r *http.Request) {
	s, _ := sessaoInterna(r.Context())
	err := tenant.EmTenant(r.Context(), h.pool, s.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(r.Context(), `UPDATE sessoes SET encerrada_em = now() WHERE id = $1 AND tenant_id = $2`, s.id, s.TenantID)
		return err
	})
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	h.apagarCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

type convite struct {
	id, tenantID, email string
	papel               Papel
}

// cadastro aceita o convite: cria usuário, TOTP (ainda não confirmado), associação e sessão pendente.
func (h *Handlers) cadastro(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Token string `json:"token"`
		Nome  string `json:"nome"`
		Senha string `json:"senha"`
	}
	if !ler(w, r, &c, false) {
		return
	}
	c.Nome = strings.TrimSpace(c.Nome)
	if campos := validarCadastro(c.Token, c.Nome, c.Senha); len(campos) > 0 {
		erroCampos(w, campos)
		return
	}
	cv, err := h.buscarConvite(r.Context(), c.Token)
	if errors.Is(err, errConviteInvalido) {
		escreverErro(w, http.StatusNotFound, "nao_encontrado")
		return
	}
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	segredo, url, t, err := h.criarUsuarioDoConvite(r, cv, c.Nome, c.Senha)
	switch {
	case errors.Is(err, errConviteInvalido):
		escreverErro(w, http.StatusNotFound, "nao_encontrado")
	case errors.Is(err, errEmailEmUso):
		escreverErro(w, http.StatusConflict, "conflito")
	case err != nil:
		falhaInterna(w, r, err)
	default:
		h.definirCookies(w, t, false)
		responder(w, http.StatusOK, map[string]string{"proximo": "2fa", "otpauth_url": url, "segredo_totp": segredo})
	}
}

func validarCadastro(token, nome, senha string) map[string]string {
	campos := map[string]string{}
	if token == "" {
		campos["token"] = "obrigatório"
	}
	if n := utf8.RuneCountInString(nome); n < 1 || n > maxNome {
		campos["nome"] = "entre 1 e " + strconv.Itoa(maxNome) + " caracteres"
	}
	if n := utf8.RuneCountInString(senha); n < minSenha || n > maxSenha {
		campos["senha"] = "entre " + strconv.Itoa(minSenha) + " e " + strconv.Itoa(maxSenha) + " caracteres"
	}
	return campos
}

func (h *Handlers) buscarConvite(ctx context.Context, token string) (convite, error) {
	var cv convite
	var papel string
	var expiraEm time.Time
	var usadoEm *time.Time
	err := h.pool.QueryRow(ctx, `SELECT convite_id, tenant_id, email, papel, expira_em, usado_em FROM auth_convite($1)`,
		hashToken(token)).Scan(&cv.id, &cv.tenantID, &cv.email, &papel, &expiraEm, &usadoEm)
	if errors.Is(err, pgx.ErrNoRows) {
		return convite{}, errConviteInvalido
	}
	if err != nil {
		return convite{}, err
	}
	if usadoEm != nil || !time.Now().Before(expiraEm) {
		return convite{}, errConviteInvalido
	}
	cv.papel = Papel(papel)
	return cv, nil
}

func (h *Handlers) criarUsuarioDoConvite(r *http.Request, cv convite, nome, senha string) (segredo, url string, t tokensSessao, err error) {
	ctx := r.Context()
	segredo, url, err = novoSegredoTOTP(cv.email)
	if err != nil {
		return "", "", t, err
	}
	usuarioID := plataforma.NovoID("usr")
	cifrado, err := CifrarTOTP(h.opcoes.ChaveCifra, usuarioID, segredo)
	if err != nil {
		return "", "", t, err
	}
	hash, err := hashSenha(senha)
	if err != nil {
		return "", "", t, err
	}
	err = tenant.EmTenant(ctx, h.pool, cv.tenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE convites SET usado_em = now()
			WHERE id = $1 AND tenant_id = $2 AND usado_em IS NULL AND expira_em > now()`, cv.id, cv.tenantID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errConviteInvalido
		}
		if _, err := tx.Exec(ctx, `INSERT INTO usuarios (id, email, nome, senha_hash) VALUES ($1, $2, $3, $4)`,
			usuarioID, cv.email, nome, hash); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return errEmailEmUso
			}
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO totp (usuario_id, segredo_cifrado) VALUES ($1, $2)`, usuarioID, cifrado); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO associacoes (usuario_id, tenant_id, papel) VALUES ($1, $2, $3)`,
			usuarioID, cv.tenantID, string(cv.papel)); err != nil {
			return err
		}
		t, err = criarSessao(ctx, tx, r, usuarioID, cv.tenantID, false)
		return err
	})
	return segredo, url, t, err
}

// permitir aplica o limite; em excesso responde 429 com Retry-After.
func (h *Handlers) permitir(w http.ResponseWriter, chave string, limite int) bool {
	ok, espera := h.limitador.Permitir(chave, limite, janelaLimite, time.Now())
	if !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(espera.Seconds())+1))
		escreverErro(w, http.StatusTooManyRequests, "limite_excedido")
	}
	return ok
}

func ipTexto(r *http.Request) string {
	if ip, ok := ipDoCliente(r); ok {
		return ip.String()
	}
	return r.RemoteAddr
}
