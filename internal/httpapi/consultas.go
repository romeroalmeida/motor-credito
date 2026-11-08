package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/romeroalmeida/motor-credito/internal/auditoria"
	"github.com/romeroalmeida/motor-credito/internal/consulta"
	"github.com/romeroalmeida/motor-credito/internal/painelauth"
	"github.com/romeroalmeida/motor-credito/internal/repositorio"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

const (
	limitePadrao = 25
	limiteMaximo = 100
)

var statusValidos = map[string]bool{"aprovado": true, "negado": true, "contraproposta": true, "documento_invalido": true, "em_analise": true}

// criarConsulta: POST /v1/consultas (o MiddlewareAPI já autenticou e devolveu o corpo intacto).
func (h handlers) criarConsulta(w http.ResponseWriter, r *http.Request) {
	t, okT := tenant.TenantDoContexto(r.Context())
	chaveID, okC := tenant.ChaveDoContexto(r.Context())
	if !okT || !okC {
		escreverErro(w, http.StatusUnauthorized, "nao_autorizado")
		return
	}
	corpo, err := io.ReadAll(r.Body)
	if err != nil {
		escreverErro(w, http.StatusBadRequest, "corpo_invalido")
		return
	}
	var req consulta.Requisicao
	if err := json.Unmarshal(corpo, &req); err != nil {
		escreverErro(w, http.StatusBadRequest, "corpo_invalido")
		return
	}
	if campos := req.Validar(); len(campos) > 0 {
		erroCampos(w, campos)
		return
	}

	chaveIdem := r.Header.Get("Idempotency-Key")
	hash := sha256.Sum256(corpo)
	if chaveIdem != "" {
		guardada, achou, err := h.Tenants.Idempotencia(r.Context(), t.ID, chaveIdem, hash[:])
		if h.erroIdempotencia(w, r, err) {
			return
		}
		if achou {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(guardada)
			return
		}
	}

	resp, err := h.Consultas.Decidir(r.Context(), t.ID, auditoria.AtorAPI(chaveID), chaveIdem, req)
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	saida, err := json.Marshal(resp)
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	if chaveIdem != "" {
		err := h.Tenants.GuardarIdempotencia(r.Context(), t.ID, chaveIdem, hash[:], saida)
		if errors.Is(err, tenant.ErrIdempotenciaConflito) {
			h.erroIdempotencia(w, r, err)
			return
		}
		if err != nil { // a consulta já está gravada; o reenvio cai na trava da própria consulta
			slog.ErrorContext(r.Context(), "guardar idempotência", "erro", err)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(saida)
}

// erroIdempotencia responde e devolve true quando há erro.
func (h handlers) erroIdempotencia(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, tenant.ErrIdempotenciaConflito):
		escreverErro(w, http.StatusConflict, "conflito_idempotencia")
	case errors.Is(err, tenant.ErrIdempotenciaChaveInvalida):
		erroCampos(w, map[string]string{"Idempotency-Key": "entre 1 e 255 caracteres"})
	default:
		falhaInterna(w, r, err)
	}
	return true
}

// listarConsultas: GET /painel/consultas com filtros e cursor.
func (h handlers) listarConsultas(w http.ResponseWriter, r *http.Request) {
	s, _ := painelauth.SessaoDoContexto(r.Context())
	q := r.URL.Query()
	f := repositorio.Filtro{Status: q.Get("status"), Faixa: q.Get("faixa"), Desde: q.Get("desde"), Ate: q.Get("ate"),
		Cursor: q.Get("cursor"), Limite: limitePadrao}
	campos := map[string]string{}
	if f.Status != "" && !statusValidos[f.Status] {
		campos["status"] = "status desconhecido"
	}
	for nome, v := range map[string]string{"desde": f.Desde, "ate": f.Ate} {
		if _, err := time.Parse("2006-01-02", v); v != "" && err != nil {
			campos[nome] = "data no formato AAAA-MM-DD"
		}
	}
	if v := q.Get("limite"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > limiteMaximo {
			campos["limite"] = "entre 1 e 100"
		}
		f.Limite = n
	}
	if doc := q.Get("documento"); doc != "" {
		hash, err := h.Consultas.HashDocumento(doc)
		if err != nil {
			campos["documento"] = "CPF ou CNPJ completo e válido"
		}
		f.DocHash = hash
	}
	if len(campos) > 0 {
		erroCampos(w, campos)
		return
	}
	itens, proximo, err := h.Consultas.Listar(r.Context(), s.TenantID, f)
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"itens": itens, "proximo_cursor": proximo})
}

// detalheConsulta: GET /painel/consultas/{id}; de outro tenant → 404.
func (h handlers) detalheConsulta(w http.ResponseWriter, r *http.Request) {
	s, _ := painelauth.SessaoDoContexto(r.Context())
	d, err := h.Consultas.Detalhe(r.Context(), s.TenantID, chi.URLParam(r, "id"))
	if errors.Is(err, repositorio.ErrNaoEncontrado) {
		escreverErro(w, http.StatusNotFound, "nao_encontrado")
		return
	}
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusOK, d)
}

// visaoGeral: GET /painel/visao-geral?periodo=hoje|7d|30d.
func (h handlers) visaoGeral(w http.ResponseWriter, r *http.Request) {
	s, _ := painelauth.SessaoDoContexto(r.Context())
	periodo := r.URL.Query().Get("periodo")
	if periodo == "" {
		periodo = "hoje"
	}
	if !repositorio.Periodos[periodo] {
		erroCampos(w, map[string]string{"periodo": "hoje, 7d ou 30d"})
		return
	}
	v, err := repositorio.ParaTenant(h.Pool, s.TenantID).VisaoGeral(r.Context(), periodo)
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusOK, v)
}
