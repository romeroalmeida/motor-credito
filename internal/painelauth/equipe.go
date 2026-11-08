package painelauth

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/romeroalmeida/motor-credito/internal/auditoria"
	"github.com/romeroalmeida/motor-credito/internal/plataforma"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

const (
	validadeConvite = 7 * 24 * time.Hour
	maxEmail        = 254
)

var (
	errJaMembroOuConvidado = errors.New("e-mail já é membro ou tem convite pendente")
	errMembroInexistente   = errors.New("membro não encontrado neste tenant")
	errUltimoAdmin         = errors.New("o tenant precisa de ao menos um admin")
)

type membroJSON struct {
	UsuarioID string    `json:"usuario_id"`
	Nome      string    `json:"nome"`
	Email     string    `json:"email"`
	Papel     Papel     `json:"papel"`
	Desde     time.Time `json:"desde"`
}

type conviteJSON struct {
	ID       string    `json:"id"`
	Email    string    `json:"email"`
	Papel    Papel     `json:"papel"`
	CriadoEm time.Time `json:"criado_em"`
	ExpiraEm time.Time `json:"expira_em"`
	Link     string    `json:"link,omitempty"` // só na criação
}

func (h *Handlers) listarEquipe(w http.ResponseWriter, r *http.Request) {
	s, _ := SessaoDoContexto(r.Context())
	membros, convites := []membroJSON{}, []conviteJSON{}
	err := tenant.EmTenant(r.Context(), h.pool, s.TenantID, func(tx pgx.Tx) error {
		var err error
		if membros, err = listarMembros(r.Context(), tx, s.TenantID); err != nil {
			return err
		}
		convites, err = listarConvitesPendentes(r.Context(), tx, s.TenantID)
		return err
	})
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"membros": membros, "convites": convites})
}

func listarMembros(ctx context.Context, tx pgx.Tx, tenantID string) ([]membroJSON, error) {
	linhas, err := tx.Query(ctx, `SELECT u.id, u.nome, u.email, a.papel, a.criado_em
		FROM associacoes a JOIN usuarios u ON u.id = a.usuario_id
		WHERE a.tenant_id = $1 ORDER BY a.criado_em`, tenantID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(linhas, func(l pgx.CollectableRow) (membroJSON, error) {
		var m membroJSON
		err := l.Scan(&m.UsuarioID, &m.Nome, &m.Email, &m.Papel, &m.Desde)
		m.Desde = m.Desde.UTC()
		return m, err
	})
}

func listarConvitesPendentes(ctx context.Context, tx pgx.Tx, tenantID string) ([]conviteJSON, error) {
	linhas, err := tx.Query(ctx, `SELECT id, email, papel, criado_em, expira_em FROM convites
		WHERE tenant_id = $1 AND usado_em IS NULL AND expira_em > now() ORDER BY criado_em`, tenantID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(linhas, func(l pgx.CollectableRow) (conviteJSON, error) {
		var c conviteJSON
		err := l.Scan(&c.ID, &c.Email, &c.Papel, &c.CriadoEm, &c.ExpiraEm)
		c.CriadoEm, c.ExpiraEm = c.CriadoEm.UTC(), c.ExpiraEm.UTC()
		return c, err
	})
}

// convidar cria o convite; o token vai só no link da resposta e o banco guarda o hash.
func (h *Handlers) convidar(w http.ResponseWriter, r *http.Request) {
	s, _ := SessaoDoContexto(r.Context())
	var c struct {
		Email string `json:"email"`
		Papel Papel  `json:"papel"`
	}
	if !ler(w, r, &c, false) {
		return
	}
	c.Email = strings.TrimSpace(c.Email)
	if campos := validarConvite(c.Email, c.Papel); len(campos) > 0 {
		erroCampos(w, campos)
		return
	}
	token, err := tokenAleatorio()
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	cv := conviteJSON{ID: plataforma.NovoID("cvt"), Email: c.Email, Papel: c.Papel}
	err = tenant.EmTenant(r.Context(), h.pool, s.TenantID, func(tx pgx.Tx) error {
		var existe bool
		err := tx.QueryRow(r.Context(), `SELECT
			EXISTS (SELECT 1 FROM associacoes a JOIN usuarios u ON u.id = a.usuario_id
			        WHERE a.tenant_id = $1 AND lower(u.email) = lower($2))
			OR EXISTS (SELECT 1 FROM convites WHERE tenant_id = $1 AND lower(email) = lower($2)
			        AND usado_em IS NULL AND expira_em > now())`, s.TenantID, c.Email).Scan(&existe)
		if err != nil {
			return err
		}
		if existe {
			return errJaMembroOuConvidado
		}
		if err := tx.QueryRow(r.Context(), `INSERT INTO convites (id, tenant_id, email, papel, token_hash, criado_por, expira_em)
			VALUES ($1, $2, $3, $4, $5, $6, now() + make_interval(secs => $7)) RETURNING criado_em, expira_em`,
			cv.ID, s.TenantID, cv.Email, string(cv.Papel), hashToken(token), s.UsuarioID, validadeConvite.Seconds()).
			Scan(&cv.CriadoEm, &cv.ExpiraEm); err != nil {
			return err
		}
		return auditoria.Registrar(r.Context(), tx, s.TenantID, auditoria.EventoConviteCriado, auditoria.AtorUsuario(s.UsuarioID),
			nil, map[string]string{"convite_id": cv.ID, "papel": string(cv.Papel)})
	})
	if errors.Is(err, errJaMembroOuConvidado) {
		responder(w, http.StatusConflict, map[string]string{"erro": "conflito", "mensagem": err.Error()})
		return
	}
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	cv.CriadoEm, cv.ExpiraEm = cv.CriadoEm.UTC(), cv.ExpiraEm.UTC()
	cv.Link = strings.TrimRight(h.opcoes.URLPainel, "/") + "/cadastro?token=" + url.QueryEscape(token)
	responder(w, http.StatusCreated, cv)
}

func validarConvite(email string, papel Papel) map[string]string {
	campos := map[string]string{}
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email || len(email) > maxEmail {
		campos["email"] = "e-mail inválido"
	}
	if !papel.valido() {
		campos["papel"] = "use admin, analista ou auditor"
	}
	return campos
}

// trocarPapel: PATCH /painel/equipe/{usuario_id} {"papel": ...}. Só no tenant da sessão; não rebaixa o último admin.
func (h *Handlers) trocarPapel(w http.ResponseWriter, r *http.Request) {
	s, _ := SessaoDoContexto(r.Context())
	var c struct {
		Papel Papel `json:"papel"`
	}
	if !ler(w, r, &c, false) {
		return
	}
	if !c.Papel.valido() {
		erroCampos(w, map[string]string{"papel": "use admin, analista ou auditor"})
		return
	}
	alvo := chi.URLParam(r, "usuario_id")
	var m membroJSON
	err := tenant.EmTenant(r.Context(), h.pool, s.TenantID, func(tx pgx.Tx) error {
		atual, err := travarMembro(r.Context(), tx, s.TenantID, alvo, c.Papel != PapelAdmin)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(r.Context(), `UPDATE associacoes a SET papel = $3 FROM usuarios u
			WHERE a.usuario_id = $1 AND a.tenant_id = $2 AND u.id = a.usuario_id
			RETURNING u.id, u.nome, u.email, a.papel, a.criado_em`, alvo, s.TenantID, string(c.Papel)).
			Scan(&m.UsuarioID, &m.Nome, &m.Email, &m.Papel, &m.Desde); err != nil {
			return err
		}
		if atual == c.Papel {
			return nil
		}
		return auditoria.Registrar(r.Context(), tx, s.TenantID, auditoria.EventoPapelAlterado, auditoria.AtorUsuario(s.UsuarioID),
			nil, map[string]string{"usuario_id": alvo, "de": string(atual), "para": string(c.Papel)})
	})
	if responderErroEquipe(w, r, err) {
		return
	}
	m.Desde = m.Desde.UTC()
	responder(w, http.StatusOK, m)
}

// removerMembro: DELETE /painel/equipe/{usuario_id}. Apaga só a associação com o tenant da sessão; as sessões
// caem na hora (auth_sessao exige a associação). Não remove o último admin.
func (h *Handlers) removerMembro(w http.ResponseWriter, r *http.Request) {
	s, _ := SessaoDoContexto(r.Context())
	alvo := chi.URLParam(r, "usuario_id")
	err := tenant.EmTenant(r.Context(), h.pool, s.TenantID, func(tx pgx.Tx) error {
		atual, err := travarMembro(r.Context(), tx, s.TenantID, alvo, true)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM associacoes WHERE usuario_id = $1 AND tenant_id = $2`, alvo, s.TenantID); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE sessoes SET encerrada_em = now()
			WHERE usuario_id = $1 AND tenant_id = $2 AND encerrada_em IS NULL`, alvo, s.TenantID); err != nil {
			return err
		}
		return auditoria.Registrar(r.Context(), tx, s.TenantID, auditoria.EventoMembroRemovido, auditoria.AtorUsuario(s.UsuarioID),
			nil, map[string]string{"usuario_id": alvo, "papel": string(atual)})
	})
	if responderErroEquipe(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// travarMembro trava os admins do tenant (FOR UPDATE: duas remoções simultâneas não zeram os admins) e devolve o
// papel atual do alvo. saiDeAdmin = a operação tira o alvo do papel admin.
func travarMembro(ctx context.Context, tx pgx.Tx, tenantID, usuarioID string, saiDeAdmin bool) (Papel, error) {
	linhas, err := tx.Query(ctx, `SELECT usuario_id FROM associacoes WHERE tenant_id = $1 AND papel = 'admin' FOR UPDATE`, tenantID)
	if err != nil {
		return "", err
	}
	admins, err := pgx.CollectRows(linhas, pgx.RowTo[string])
	if err != nil {
		return "", err
	}
	var papel Papel
	err = tx.QueryRow(ctx, `SELECT papel FROM associacoes WHERE usuario_id = $1 AND tenant_id = $2 FOR UPDATE`,
		usuarioID, tenantID).Scan(&papel)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errMembroInexistente
	}
	if err != nil {
		return "", err
	}
	if saiDeAdmin && papel == PapelAdmin && len(admins) <= 1 {
		return "", errUltimoAdmin
	}
	return papel, nil
}

func responderErroEquipe(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, errMembroInexistente):
		escreverErro(w, http.StatusNotFound, "nao_encontrado")
	case errors.Is(err, errUltimoAdmin):
		responder(w, http.StatusConflict, map[string]string{"erro": "ultimo_admin", "mensagem": errUltimoAdmin.Error()})
	default:
		falhaInterna(w, r, err)
	}
	return true
}
