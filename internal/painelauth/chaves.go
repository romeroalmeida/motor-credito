package painelauth

import (
	"errors"
	"net/http"
	"net/netip"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

const maxNomeChave = 80

type chaveJSON struct {
	ID          string     `json:"id"`
	Nome        string     `json:"nome"`
	Prefixo     string     `json:"prefixo"`
	CriadaEm    time.Time  `json:"criada_em"`
	RevogadaEm  *time.Time `json:"revogada_em"`
	UltimoUsoEm *time.Time `json:"ultimo_uso_em"`
}

// chaveCriadaJSON: chave = "ck_live_<id>" (vai no Bearer); segredo = chave do HMAC, mostrado uma vez só.
type chaveCriadaJSON struct {
	ID      string `json:"id"`
	Chave   string `json:"chave"`
	Segredo string `json:"segredo"`
}

type ipsJSON struct {
	IPs []string `json:"ips"`
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func (h *Handlers) listarChaves(w http.ResponseWriter, r *http.Request) {
	s, _ := SessaoDoContexto(r.Context())
	chaves, err := h.tenants.ListarChaves(r.Context(), s.TenantID)
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	itens := make([]chaveJSON, len(chaves))
	for i, c := range chaves {
		itens[i] = chaveJSON{ID: c.ID, Nome: c.Nome, Prefixo: c.Prefixo(), CriadaEm: c.CriadaEm.UTC(),
			RevogadaEm: utc(c.RevogadaEm), UltimoUsoEm: utc(c.UltimoUsoEm)}
	}
	responder(w, http.StatusOK, map[string][]chaveJSON{"itens": itens})
}

func (h *Handlers) criarChave(w http.ResponseWriter, r *http.Request) {
	s, _ := SessaoDoContexto(r.Context())
	var c struct {
		Nome string `json:"nome"`
	}
	if !ler(w, r, &c, true) {
		return
	}
	if utf8.RuneCountInString(c.Nome) > maxNomeChave {
		erroCampos(w, map[string]string{"nome": "no máximo 80 caracteres"})
		return
	}
	criada, err := h.tenants.CriarChaveComNome(r.Context(), s.TenantID, c.Nome)
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusCreated, chaveCriadaJSON(criada))
}

func (h *Handlers) revogarChave(w http.ResponseWriter, r *http.Request) {
	s, _ := SessaoDoContexto(r.Context())
	err := h.tenants.RevogarChave(r.Context(), s.TenantID, chi.URLParam(r, "id"))
	if errors.Is(err, tenant.ErrNaoEncontrado) {
		escreverErro(w, http.StatusNotFound, "nao_encontrado")
		return
	}
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) girarChave(w http.ResponseWriter, r *http.Request) {
	s, _ := SessaoDoContexto(r.Context())
	criada, err := h.tenants.GirarChave(r.Context(), s.TenantID, chi.URLParam(r, "id"))
	if errors.Is(err, tenant.ErrNaoEncontrado) {
		escreverErro(w, http.StatusNotFound, "nao_encontrado")
		return
	}
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusCreated, chaveCriadaJSON(criada))
}

func (h *Handlers) lerIPs(w http.ResponseWriter, r *http.Request) {
	s, _ := SessaoDoContexto(r.Context())
	t, err := h.tenants.Buscar(r.Context(), s.TenantID)
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusOK, ipsParaJSON(t.IPsPermitidos))
}

func (h *Handlers) definirIPs(w http.ResponseWriter, r *http.Request) {
	s, _ := SessaoDoContexto(r.Context())
	var c struct {
		IPs *[]string `json:"ips"`
	}
	if !ler(w, r, &c, false) {
		return
	}
	if c.IPs == nil {
		erroCampos(w, map[string]string{"ips": "obrigatório"})
		return
	}
	prefixos, err := h.tenants.DefinirIPs(r.Context(), s.TenantID, *c.IPs)
	if errors.Is(err, tenant.ErrIPInvalido) {
		erroCampos(w, map[string]string{"ips": err.Error()})
		return
	}
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusOK, ipsParaJSON(prefixos))
}

func ipsParaJSON(prefixos []netip.Prefix) ipsJSON {
	ips := make([]string, len(prefixos))
	for i, p := range prefixos {
		ips[i] = p.String()
	}
	return ipsJSON{IPs: ips}
}
