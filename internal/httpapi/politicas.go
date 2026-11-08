package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/romeroalmeida/motor-credito/internal/consulta"
	"github.com/romeroalmeida/motor-credito/internal/painelauth"
	"github.com/romeroalmeida/motor-credito/internal/politica"
	"github.com/romeroalmeida/motor-credito/internal/repositorio"
)

const maxComentario = 500

func (h handlers) repo(r *http.Request) (*repositorio.Repo, painelauth.Sessao) {
	s, _ := painelauth.SessaoDoContexto(r.Context())
	return repositorio.ParaTenant(h.Pool, s.TenantID), s
}

// compilar responde 422 com cada regra e motivo quando a política não compila.
func compilar(w http.ResponseWriter, r *http.Request, p politica.Politica) (*politica.Compilada, bool) {
	c, err := politica.Compilar(p)
	if err == nil {
		return c, true
	}
	var invalida *politica.ErroPolitica
	if errors.As(err, &invalida) {
		responder(w, http.StatusUnprocessableEntity, erroAPI{Erro: "politica_invalida", Detalhes: invalida.Problemas})
		return nil, false
	}
	falhaInterna(w, r, err)
	return nil, false
}

func (h handlers) estadoPolitica(w http.ResponseWriter, r *http.Request) {
	repo, _ := h.repo(r)
	e, err := repo.Politica(r.Context())
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusOK, e)
}

func (h handlers) salvarRascunho(w http.ResponseWriter, r *http.Request) {
	repo, s := h.repo(r)
	var p politica.Politica
	if !lerJSON(w, r, &p) {
		return
	}
	if len(p.Faixas) == 0 {
		erroCampos(w, map[string]string{"faixas": "ao menos uma faixa"})
		return
	}
	e, err := repo.SalvarRascunho(r.Context(), p, s.UsuarioID)
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusOK, e)
}

// testarPolitica roda a política (enviada ou o rascunho salvo) contra um documento; não grava consulta.
func (h handlers) testarPolitica(w http.ResponseWriter, r *http.Request) {
	repo, _ := h.repo(r)
	var c struct {
		Documento string             `json:"documento"`
		Pedido    *consulta.Pedido   `json:"pedido"`
		Dados     consulta.Dados     `json:"dados"`
		Politica  *politica.Politica `json:"politica"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	req := consulta.Requisicao{Documento: c.Documento, Pedido: c.Pedido, Dados: c.Dados}
	if campos := consulta.ValidarSimulacao(req); len(campos) > 0 {
		erroCampos(w, campos)
		return
	}
	if c.Politica == nil {
		e, err := repo.Politica(r.Context())
		if err != nil {
			falhaInterna(w, r, err)
			return
		}
		c.Politica = &e.Rascunho
	}
	compilada, ok := compilar(w, r, *c.Politica)
	if !ok {
		return
	}
	responder(w, http.StatusOK, h.Consultas.Simular(r.Context(), req, compilada))
}

type confirmacao struct {
	Confirmacao bool   `json:"confirmacao"`
	Comentario  string `json:"comentario"`
}

func lerConfirmacao(w http.ResponseWriter, r *http.Request) (confirmacao, bool) {
	var c confirmacao
	if !lerJSON(w, r, &c) {
		return c, false
	}
	campos := map[string]string{}
	if !c.Confirmacao {
		campos["confirmacao"] = "deve ser true"
	}
	if utf8.RuneCountInString(c.Comentario) > maxComentario {
		campos["comentario"] = "até 500 caracteres"
	}
	if len(campos) > 0 {
		erroCampos(w, campos)
		return c, false
	}
	return c, true
}

// publicar compila o rascunho e grava como versão N+1.
func (h handlers) publicar(w http.ResponseWriter, r *http.Request) {
	repo, s := h.repo(r)
	c, ok := lerConfirmacao(w, r)
	if !ok {
		return
	}
	e, err := repo.Politica(r.Context())
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	if _, ok := compilar(w, r, e.Rascunho); !ok {
		return
	}
	v, err := repo.Publicar(r.Context(), e.Rascunho, s.UsuarioID, c.Comentario, nil)
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusCreated, v)
}

// reverter publica uma cópia da versão pedida como versão nova.
func (h handlers) reverter(w http.ResponseWriter, r *http.Request) {
	repo, s := h.repo(r)
	n, ok := versaoDaRota(w, r)
	if !ok {
		return
	}
	if _, ok := lerConfirmacao(w, r); !ok {
		return
	}
	antiga, ok := h.lerVersao(w, r, repo, n)
	if !ok {
		return
	}
	if _, ok := compilar(w, r, *antiga.Conteudo); !ok {
		return
	}
	v, err := repo.Publicar(r.Context(), *antiga.Conteudo, s.UsuarioID, "reversão para a versão "+strconv.Itoa(n), &n)
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusCreated, v)
}

func (h handlers) versoes(w http.ResponseWriter, r *http.Request) {
	repo, _ := h.repo(r)
	itens, err := repo.Versoes(r.Context())
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"itens": itens})
}

func (h handlers) versao(w http.ResponseWriter, r *http.Request) {
	repo, _ := h.repo(r)
	n, ok := versaoDaRota(w, r)
	if !ok {
		return
	}
	if v, ok := h.lerVersao(w, r, repo, n); ok {
		responder(w, http.StatusOK, v)
	}
}

// diferenca: GET /painel/politicas/diferenca?de=N&para=M|rascunho.
func (h handlers) diferenca(w http.ResponseWriter, r *http.Request) {
	repo, _ := h.repo(r)
	q := r.URL.Query()
	de, err := strconv.Atoi(q.Get("de"))
	para := q.Get("para")
	campos := map[string]string{}
	if err != nil || de < 1 {
		campos["de"] = "número de versão"
	}
	paraN, errPara := strconv.Atoi(para)
	if para != "rascunho" && (errPara != nil || paraN < 1) {
		campos["para"] = `número de versão ou "rascunho"`
	}
	if len(campos) > 0 {
		erroCampos(w, campos)
		return
	}
	vDe, ok := h.lerVersao(w, r, repo, de)
	if !ok {
		return
	}
	var destino politica.Politica
	if para == "rascunho" {
		e, err := repo.Politica(r.Context())
		if err != nil {
			falhaInterna(w, r, err)
			return
		}
		destino = e.Rascunho
	} else {
		vPara, ok := h.lerVersao(w, r, repo, paraN)
		if !ok {
			return
		}
		destino = *vPara.Conteudo
	}
	mudancas, err := diferencas(*vDe.Conteudo, destino)
	if err != nil {
		falhaInterna(w, r, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"de": de, "para": para, "mudancas": mudancas})
}

func versaoDaRota(w http.ResponseWriter, r *http.Request) (int, bool) {
	n, err := strconv.Atoi(chi.URLParam(r, "versao"))
	if err != nil || n < 1 {
		erroCampos(w, map[string]string{"versao": "número de versão"})
		return 0, false
	}
	return n, true
}

func (h handlers) lerVersao(w http.ResponseWriter, r *http.Request, repo *repositorio.Repo, n int) (repositorio.Versao, bool) {
	v, err := repo.Versao(r.Context(), n)
	if errors.Is(err, repositorio.ErrNaoEncontrado) {
		escreverErro(w, http.StatusNotFound, "nao_encontrado")
		return v, false
	}
	if err != nil {
		falhaInterna(w, r, err)
		return v, false
	}
	return v, true
}
