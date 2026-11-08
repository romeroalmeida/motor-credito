package painelauth

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

const maxCorpoPainel = 16 << 10

type erroResposta struct {
	Erro   string            `json:"erro"`
	Campos map[string]string `json:"campos,omitempty"`
}

func responder(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func escreverErro(w http.ResponseWriter, status int, codigo string) {
	responder(w, status, erroResposta{Erro: codigo})
}

func erroCampos(w http.ResponseWriter, campos map[string]string) {
	responder(w, http.StatusBadRequest, erroResposta{Erro: "corpo_invalido", Campos: campos})
}

func falhaInterna(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "painel", "rota", r.URL.Path, "erro", err)
	escreverErro(w, http.StatusInternalServerError, "erro_interno")
}

// ler decodifica o JSON do corpo (até 16 KB). Corpo vazio é aceito quando opcional.
func ler(w http.ResponseWriter, r *http.Request, destino any, opcional bool) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCorpoPainel)).Decode(destino)
	if err == nil || (opcional && errors.Is(err, io.EOF)) {
		return true
	}
	escreverErro(w, http.StatusBadRequest, "corpo_invalido")
	return false
}
