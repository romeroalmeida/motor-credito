package httpapi_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/romeroalmeida/motor-credito/internal/bureau"
	"github.com/romeroalmeida/motor-credito/internal/plataforma"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

type saidaSimulador struct {
	Requisicao struct {
		Metodo     string            `json:"metodo"`
		URL        string            `json:"url"`
		Cabecalhos map[string]string `json:"cabecalhos"`
		Corpo      string            `json:"corpo"`
	} `json:"requisicao"`
	Resposta struct {
		Status int             `json:"status"`
		Corpo  json.RawMessage `json:"corpo"`
	} `json:"resposta"`
	ConsultaID *string `json:"consulta_id"`
	Trechos    struct {
		Curl, Node, Python string
	} `json:"trechos"`
}

// Corpo com aspa simples, aspas duplas, acento e < >: o que costuma quebrar o escape dos trechos.
const requisicaoSimulada = `{"documento":"` + bureau.CPFScoreAlto + `","finalidade":"concessao_credito",
	"pedido":{"valor":1000000,"prazo":24},"dados":{"nome":"Joana D'Ávila \"Jô\" <teste>"}}`

func entradaSimulador(chaveID, idem, requisicao string) string {
	return `{"chave_id":"` + chaveID + `","idempotency_key":"` + idem + `","requisicao":` + requisicao + `}`
}

// hmacManual reimplementa a fórmula dos trechos Node/Python (hex HMAC-SHA256 de ts + "." + idem + "." + corpo),
// sem passar por tenant.Assinar.
func hmacManual(segredo, ts, idem, corpo string) string {
	m := hmac.New(sha256.New, []byte(segredo))
	m.Write([]byte(ts + "." + idem + "." + corpo))
	return hex.EncodeToString(m.Sum(nil))
}

func TestSimulador(t *testing.T) {
	a := novoAmbiente(t)
	var log bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&log, nil)))
	t.Cleanup(func() { slog.SetDefault(anterior) })
	var respostas []string
	simular := func(s sessao, corpo string) (int, string) {
		w := a.painel(s, http.MethodPost, "/painel/simulador/consultas", corpo)
		respostas = append(respostas, w.Body.String())
		return w.Code, w.Body.String()
	}
	simularOK := func(t *testing.T, sess sessao, entrada string) saidaSimulador {
		t.Helper()
		status, corpo := simular(sess, entrada)
		if status != http.StatusOK {
			t.Fatalf("status %d: %s", status, corpo)
		}
		var s saidaSimulador
		if err := json.Unmarshal([]byte(corpo), &s); err != nil {
			t.Fatalf("json: %v: %s", err, corpo)
		}
		return s
	}

	t.Run("admin simula e a consulta fica gravada com a decisão e o autor", func(t *testing.T) {
		s := simularOK(t, a.sessA, entradaSimulador(a.chaveA.ID, "", requisicaoSimulada))
		if s.Resposta.Status != http.StatusOK || s.ConsultaID == nil {
			t.Fatalf("resposta %d, consulta_id %v: %s", s.Resposta.Status, s.ConsultaID, s.Resposta.Corpo)
		}
		var decisao struct{ ID, Status string }
		_ = json.Unmarshal(s.Resposta.Corpo, &decisao)
		if decisao.Status != "aprovado" || decisao.ID != *s.ConsultaID {
			t.Fatalf("decisão %+v", decisao)
		}
		if n := a.contar(`SELECT count(*) FROM consultas WHERE id = $1 AND tenant_id = $2 AND status = 'aprovado'`,
			*s.ConsultaID, a.tenantA); n != 1 {
			t.Fatalf("consulta gravada: %d", n)
		}
		var usuario string
		if err := a.banco.Dono.QueryRow(context.Background(), `SELECT usuario_id FROM sessoes WHERE token_hash = $1`,
			hash(a.sessA.token)).Scan(&usuario); err != nil {
			t.Fatal(err)
		}
		if n := a.contar(`SELECT count(*) FROM auditoria WHERE tenant_id = $1 AND evento = 'simulador.consulta'
			AND ator = $2 AND consulta_id = $3 AND dados->>'via' = 'simulador' AND dados->>'chave_id' = $4`,
			a.tenantA, "usuario:"+usuario, *s.ConsultaID, a.chaveA.ID); n != 1 {
			t.Fatalf("auditoria com o usuário que disparou: %d", n)
		}

		cab := s.Requisicao.Cabecalhos
		idem := cab["Idempotency-Key"]
		if s.Requisicao.Metodo != http.MethodPost || !strings.HasSuffix(s.Requisicao.URL, "/v1/consultas") ||
			cab["Authorization"] != "Bearer "+a.chaveA.Chave || cab["Content-Type"] != "application/json" ||
			!regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(idem) {
			t.Fatalf("requisição: %+v", s.Requisicao)
		}
		if cab["X-Assinatura"] != hmacManual(a.chaveA.Segredo, cab["X-Timestamp"], idem, s.Requisicao.Corpo) {
			t.Fatal("X-Assinatura não é o HMAC de ts.idem.corpo")
		}

		// O corpo embutido em cada trecho é exatamente o assinado: com ele, a fórmula dos trechos bate com Assinar.
		corpos := map[string]string{
			"node":   extrairLiteral(t, s.Trechos.Node, `(?m)^const corpo = (.*)$`),
			"python": extrairLiteral(t, s.Trechos.Python, `(?m)^corpo = (.*)$`),
			"curl":   extrairShell(t, s.Trechos.Curl),
		}
		for linguagem, corpo := range corpos {
			if corpo != s.Requisicao.Corpo {
				t.Errorf("%s: corpo %q, quer %q", linguagem, corpo, s.Requisicao.Corpo)
			}
			ts, id := "1700000000", "idem-trecho"
			if hmacManual(a.chaveA.Segredo, ts, id, corpo) != tenant.Assinar(a.chaveA.Segredo, ts, id, []byte(s.Requisicao.Corpo)) {
				t.Errorf("%s: assinatura do trecho diverge de tenant.Assinar", linguagem)
			}
		}
		for linguagem, trecho := range map[string]string{"curl": s.Trechos.Curl, "node": s.Trechos.Node, "python": s.Trechos.Python} {
			if !strings.Contains(trecho, "MOTOR_SEGREDO") || !strings.Contains(trecho, a.chaveA.Chave) {
				t.Errorf("%s: trecho sem MOTOR_SEGREDO ou sem o id da chave:\n%s", linguagem, trecho)
			}
		}
	})

	t.Run("mesma idempotency_key devolve a mesma decisão", func(t *testing.T) {
		corpo := entradaSimulador(a.chaveA.ID, "simulador-idem-1", requisicaoSimulada)
		um := simularOK(t, a.sessA, corpo)
		dois := simularOK(t, a.sessA, corpo)
		if um.ConsultaID == nil || dois.ConsultaID == nil || *um.ConsultaID != *dois.ConsultaID ||
			!bytes.Equal(um.Resposta.Corpo, dois.Resposta.Corpo) {
			t.Fatalf("reenvio mudou a decisão: %v × %v", um.ConsultaID, dois.ConsultaID)
		}
	})

	t.Run("corpo inválido mostra o 400 real da /v1", func(t *testing.T) {
		s := simularOK(t, a.sessA, entradaSimulador(a.chaveA.ID, "", `{"documento":"`+bureau.CPFScoreAlto+`"}`))
		if s.Resposta.Status != http.StatusBadRequest || !strings.Contains(string(s.Resposta.Corpo), "corpo_invalido") || s.ConsultaID != nil {
			t.Fatalf("resposta %d %s", s.Resposta.Status, s.Resposta.Corpo)
		}
	})

	t.Run("IP fora da lista da chave recebe o 401 real", func(t *testing.T) {
		a.dono(`UPDATE tenants SET ips_permitidos = '{10.0.0.0/8}' WHERE id = $1`, a.tenantA)
		defer a.dono(`UPDATE tenants SET ips_permitidos = '{}' WHERE id = $1`, a.tenantA)
		s := simularOK(t, a.sessA, entradaSimulador(a.chaveA.ID, "", requisicaoSimulada))
		if s.Resposta.Status != http.StatusUnauthorized {
			t.Fatalf("resposta %d %s", s.Resposta.Status, s.Resposta.Corpo)
		}
	})

	t.Run("recusas", func(t *testing.T) {
		revogada, err := tenant.NovoServico(a.banco.App, chaveCifra).CriarChaveComNome(context.Background(), a.tenantA, "velha", "sistema")
		if err != nil {
			t.Fatal(err)
		}
		if err := tenant.NovoServico(a.banco.App, chaveCifra).RevogarChave(context.Background(), a.tenantA, revogada.ID, "sistema"); err != nil {
			t.Fatal(err)
		}
		analista := sessao{token: "tok-analista", csrf: "csrf-analista"}
		usr := plataforma.NovoID("usr")
		a.dono(`INSERT INTO usuarios (id, email, nome, senha_hash) VALUES ($1, $1 || '@t.local', 'Analista', 'x')`, usr)
		a.dono(`INSERT INTO associacoes (usuario_id, tenant_id, papel) VALUES ($1, $2, 'analista')`, usr, a.tenantA)
		a.dono(`INSERT INTO sessoes (id, token_hash, csrf_hash, usuario_id, tenant_id, dois_fatores_em, expira_em)
			VALUES ($1, $2, $3, $4, $5, now(), now() + interval '1 hour')`,
			plataforma.NovoID("ses"), hash(analista.token), hash(analista.csrf), usr, a.tenantA)
		semCSRF := sessao{token: a.sessA.token, csrf: ""}

		casos := []struct {
			nome   string
			s      sessao
			chave  string
			status int
			erro   string
		}{
			{"analista", analista, a.chaveA.ID, http.StatusForbidden, "proibido"},
			{"auditor", a.sessAuditorA, a.chaveA.ID, http.StatusForbidden, "proibido"},
			{"sem CSRF", semCSRF, a.chaveA.ID, http.StatusForbidden, "csrf_invalido"},
			{"chave de outro tenant", a.sessB, a.chaveA.ID, http.StatusNotFound, "nao_encontrado"},
			{"chave revogada", a.sessA, revogada.ID, http.StatusNotFound, "nao_encontrado"},
			{"chave inexistente", a.sessA, "01JC8ZK3Q0NAOEXISTE0000000", http.StatusNotFound, "nao_encontrado"},
			{"sem chave", a.sessA, "", http.StatusBadRequest, "corpo_invalido"},
		}
		antes := a.contar(`SELECT count(*) FROM consultas`)
		for _, c := range casos {
			status, corpo := simular(c.s, entradaSimulador(c.chave, "", requisicaoSimulada))
			if status != c.status || !strings.Contains(corpo, c.erro) {
				t.Errorf("%s: %d %s; quer %d %s", c.nome, status, corpo, c.status, c.erro)
			}
		}
		if depois := a.contar(`SELECT count(*) FROM consultas`); depois != antes {
			t.Fatalf("recusa gravou consulta: %d → %d", antes, depois)
		}
	})

	t.Run("o segredo não aparece na resposta nem no log", func(t *testing.T) {
		if len(respostas) < 5 {
			t.Fatalf("poucas respostas varridas: %d", len(respostas))
		}
		for _, r := range append(respostas, log.String()) {
			if strings.Contains(r, a.chaveA.Segredo) {
				t.Fatalf("segredo vazou: %s", r)
			}
		}
	})
}

func extrairLiteral(t *testing.T, trecho, padrao string) string {
	t.Helper()
	m := regexp.MustCompile(padrao).FindStringSubmatch(trecho)
	if m == nil {
		t.Fatalf("corpo não achado em:\n%s", trecho)
	}
	var v string
	if err := json.Unmarshal([]byte(m[1]), &v); err != nil {
		t.Fatalf("literal %s: %v", m[1], err)
	}
	return v
}

// extrairShell lê CORPO='...' desfazendo o escape '\” da aspa simples.
func extrairShell(t *testing.T, trecho string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^CORPO='(.*)'$`).FindStringSubmatch(trecho)
	if m == nil {
		t.Fatalf("CORPO não achado em:\n%s", trecho)
	}
	return strings.ReplaceAll(m[1], `'\''`, "'")
}
