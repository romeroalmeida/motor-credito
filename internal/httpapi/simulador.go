package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/romeroalmeida/motor-credito/internal/auditoria"
	"github.com/romeroalmeida/motor-credito/internal/painelauth"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

const caminhoConsultas = "/v1/consultas"

type entradaSimulador struct {
	ChaveID        string          `json:"chave_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Requisicao     json.RawMessage `json:"requisicao"`
}

type requisicaoSimulada struct {
	Metodo     string            `json:"metodo"`
	URL        string            `json:"url"`
	Cabecalhos map[string]string `json:"cabecalhos"`
	Corpo      string            `json:"corpo"`
}

type respostaSimulada struct {
	Status     int               `json:"status"`
	Cabecalhos map[string]string `json:"cabecalhos"`
	Corpo      json.RawMessage   `json:"corpo"`
	DuracaoMs  int64             `json:"duracao_ms"`
}

type saidaSimulador struct {
	Requisicao requisicaoSimulada `json:"requisicao"`
	Resposta   respostaSimulada   `json:"resposta"`
	ConsultaID *string            `json:"consulta_id"`
	Trechos    trechos            `json:"trechos"`
}

// cabecalhosDevolvidos: o que da resposta real interessa a quem integra (o resto é igual em toda rota).
var cabecalhosDevolvidos = []string{"Content-Type", "Retry-After"}

// simular: POST /painel/simulador/consultas (só admin). Assina com a chave escolhida do tenant da sessão e roda a
// requisição pelo router inteiro (api), em processo: mesmo MiddlewareAPI, limite e lista de IPs da /v1. O IP de
// origem é o do usuário do painel. Erro da /v1 (400, 401, 429…) volta dentro de resposta, com 200 aqui.
func (h handlers) simular(api http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, _ := painelauth.SessaoDoContexto(r.Context())
		var e entradaSimulador
		if !lerJSON(w, r, &e) {
			return
		}
		var corpo bytes.Buffer
		campos := map[string]string{}
		if e.ChaveID == "" {
			campos["chave_id"] = "obrigatório"
		}
		if err := json.Compact(&corpo, e.Requisicao); err != nil || !strings.HasPrefix(corpo.String(), "{") {
			campos["requisicao"] = "objeto JSON da consulta (o mesmo corpo da /v1/consultas)"
		}
		if len(campos) > 0 {
			erroCampos(w, campos)
			return
		}
		idem := e.IdempotencyKey
		if idem == "" {
			var err error
			if idem, err = novoUUID(); err != nil {
				falhaInterna(w, r, err)
				return
			}
		}

		ts := strconv.FormatInt(time.Now().Unix(), 10)
		assinatura, err := h.Tenants.AssinarComChave(r.Context(), s.TenantID, e.ChaveID, ts, idem, corpo.Bytes())
		if errors.Is(err, tenant.ErrNaoEncontrado) {
			escreverErro(w, http.StatusNotFound, "nao_encontrado")
			return
		}
		if err != nil {
			falhaInterna(w, r, err)
			return
		}
		cab := map[string]string{
			"Authorization":   "Bearer " + tenant.Chave{ID: e.ChaveID}.Prefixo(),
			"Content-Type":    "application/json",
			"X-Timestamp":     ts,
			"Idempotency-Key": idem,
			"X-Assinatura":    assinatura,
		}
		resp, consultaID, err := executar(r, api, cab, corpo.Bytes())
		if err != nil {
			falhaInterna(w, r, err)
			return
		}
		// Quem disparou fica na cadeia (a consulta em si registra só a chave, como qualquer chamada da API).
		err = tenant.EmTenant(r.Context(), h.Pool, s.TenantID, func(tx pgx.Tx) error {
			return auditoria.Registrar(r.Context(), tx, s.TenantID, auditoria.EventoSimuladorConsulta,
				auditoria.AtorUsuario(s.UsuarioID), consultaID,
				map[string]any{"chave_id": e.ChaveID, "via": "simulador", "status_http": resp.Status})
		})
		if err != nil {
			falhaInterna(w, r, err)
			return
		}
		url := h.urlConsultas(r)
		responder(w, http.StatusOK, saidaSimulador{
			Requisicao: requisicaoSimulada{Metodo: http.MethodPost, URL: url, Cabecalhos: cab, Corpo: corpo.String()},
			Resposta:   resp,
			ConsultaID: consultaID,
			Trechos:    gerarTrechos(url, cab["Authorization"], corpo.String()),
		})
	}
}

// executar roda a requisição assinada no router. O contexto é novo (sem a sessão nem o roteamento do chi da
// requisição do painel); só o cancelamento é herdado.
func executar(r *http.Request, api http.Handler, cab map[string]string, corpo []byte) (respostaSimulada, *string, error) {
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	defer context.AfterFunc(r.Context(), cancelar)()
	interna, err := http.NewRequestWithContext(ctx, http.MethodPost, caminhoConsultas, bytes.NewReader(corpo))
	if err != nil {
		return respostaSimulada{}, nil, err
	}
	interna.RemoteAddr = r.RemoteAddr // já reescrito pelo IPReal: a lista de IPs da chave vale para o usuário
	for nome, valor := range cab {
		interna.Header.Set(nome, valor)
	}
	gravador := httptest.NewRecorder()
	inicio := time.Now()
	api.ServeHTTP(gravador, interna)
	resp := respostaSimulada{Status: gravador.Code, Cabecalhos: map[string]string{},
		DuracaoMs: time.Since(inicio).Milliseconds()}
	for _, nome := range cabecalhosDevolvidos {
		if v := gravador.Header().Get(nome); v != "" {
			resp.Cabecalhos[nome] = v
		}
	}
	resp.Corpo = bytes.TrimSpace(gravador.Body.Bytes())
	if !json.Valid(resp.Corpo) {
		texto, _ := json.Marshal(string(resp.Corpo))
		resp.Corpo = texto
	}
	var decisao struct {
		ID string `json:"id"`
	}
	if resp.Status == http.StatusOK && json.Unmarshal(resp.Corpo, &decisao) == nil && decisao.ID != "" {
		return resp, &decisao.ID, nil
	}
	return resp, nil, nil
}

// urlConsultas: URL pública da API (URL_PAINEL; o nginx serve painel e /v1 na mesma origem).
// shortcut: sem URL_PAINEL deriva do Host, que atrás de proxy pode vir sem porta; configure URL_PAINEL em produção.
func (h handlers) urlConsultas(r *http.Request) string {
	if h.URLPublica != "" {
		return strings.TrimRight(h.URLPublica, "/") + caminhoConsultas
	}
	esquema := "http"
	if r.TLS != nil {
		esquema = "https"
	}
	return esquema + "://" + r.Host + caminhoConsultas
}

// novoUUID gera um UUID v4 (RFC 9562) para a Idempotency-Key que o usuário deixou em branco.
func novoUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("gerar idempotency key: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
