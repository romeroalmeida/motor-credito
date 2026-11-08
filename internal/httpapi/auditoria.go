package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/romeroalmeida/motor-credito/internal/auditoria"
	"github.com/romeroalmeida/motor-credito/internal/painelauth"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

// listarAuditoria: GET /painel/auditoria?cursor=<seq>&limite=N, mais novo primeiro.
func (h handlers) listarAuditoria(w http.ResponseWriter, r *http.Request) {
	s, _ := painelauth.SessaoDoContexto(r.Context())
	q := r.URL.Query()
	campos := map[string]string{}
	antesDe, limite := int64(0), limitePadrao
	if v := q.Get("cursor"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			campos["cursor"] = "valor de proximo_cursor"
		}
		antesDe = n
	}
	if v := q.Get("limite"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > limiteMaximo {
			campos["limite"] = "entre 1 e 100"
		}
		limite = n
	}
	if len(campos) > 0 {
		erroCampos(w, campos)
		return
	}
	var itens []auditoria.Registro
	err := tenant.EmTenant(r.Context(), h.Pool, s.TenantID, func(tx pgx.Tx) (err error) {
		itens, err = auditoria.Listar(r.Context(), tx, s.TenantID, antesDe, limite)
		return err
	})
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	var proximo *string
	if len(itens) == limite && itens[len(itens)-1].Seq > 1 {
		c := strconv.FormatInt(itens[len(itens)-1].Seq, 10)
		proximo = &c
	}
	responder(w, http.StatusOK, map[string]any{"itens": itens, "proximo_cursor": proximo})
}

// integridadeAuditoria: GET /painel/auditoria/integridade percorre a cadeia inteira do tenant.
func (h handlers) integridadeAuditoria(w http.ResponseWriter, r *http.Request) {
	s, _ := painelauth.SessaoDoContexto(r.Context())
	err := tenant.EmTenant(r.Context(), h.Pool, s.TenantID, func(tx pgx.Tx) error {
		return auditoria.Verificar(r.Context(), tx, s.TenantID)
	})
	switch {
	case err == nil:
		responder(w, http.StatusOK, map[string]any{"integra": true})
	case errors.Is(err, auditoria.ErrCadeiaQuebrada):
		responder(w, http.StatusOK, map[string]any{"integra": false, "mensagem": err.Error()})
	default:
		falhaInterna(w, r, err)
	}
}
