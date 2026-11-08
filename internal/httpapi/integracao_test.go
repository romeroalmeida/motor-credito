package httpapi_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/romeroalmeida/motor-credito/internal/auditoria"
	"github.com/romeroalmeida/motor-credito/internal/bureau"
	"github.com/romeroalmeida/motor-credito/internal/consulta"
	"github.com/romeroalmeida/motor-credito/internal/httpapi"
	"github.com/romeroalmeida/motor-credito/internal/painelauth"
	"github.com/romeroalmeida/motor-credito/internal/plataforma"
	"github.com/romeroalmeida/motor-credito/internal/plataforma/bancoteste"
	"github.com/romeroalmeida/motor-credito/internal/politica"
	"github.com/romeroalmeida/motor-credito/internal/repositorio"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

var (
	chaveHMAC  = bytes.Repeat([]byte{7}, 32)
	chaveCifra = bytes.Repeat([]byte{9}, 32)
)

type sessao struct{ token, csrf string }

type ambiente struct {
	t                *testing.T
	banco            *bancoteste.Banco
	h                http.Handler
	tenantA, tenantB string
	chaveA           string // "ck_live_<id>_<segredo>" do tenant A
	sessA, sessB     sessao
}

func novoAmbiente(t *testing.T) *ambiente {
	b := bancoteste.Novo(t)
	ctx := context.Background()
	a := &ambiente{t: t, banco: b, tenantA: plataforma.NovoID("ten"), tenantB: plataforma.NovoID("ten")}
	tenants := tenant.NovoServico(b.App)
	for _, tc := range []struct {
		id   string
		sess *sessao
	}{{a.tenantA, &a.sessA}, {a.tenantB, &a.sessB}} {
		usr := plataforma.NovoID("usr")
		*tc.sess = sessao{token: "tok-" + tc.id, csrf: "csrf-" + tc.id}
		a.dono(`INSERT INTO tenants (id, nome) VALUES ($1, $1)`, tc.id)
		a.dono(`INSERT INTO usuarios (id, email, nome, senha_hash) VALUES ($1, $1 || '@t.local', 'Admin ' || $1, 'x')`, usr)
		a.dono(`INSERT INTO associacoes (usuario_id, tenant_id, papel) VALUES ($1, $2, 'admin')`, usr, tc.id)
		a.dono(`INSERT INTO sessoes (id, token_hash, csrf_hash, usuario_id, tenant_id, dois_fatores_em, expira_em)
			VALUES ($1, $2, $3, $4, $5, now(), now() + interval '1 hour')`,
			plataforma.NovoID("ses"), hash(tc.sess.token), hash(tc.sess.csrf), usr, tc.id)
		if _, err := repositorio.ParaTenant(b.App, tc.id).Publicar(ctx, politica.ModeloEmprestimoPF(), usr, "inicial", nil); err != nil {
			t.Fatalf("publicar política inicial: %v", err)
		}
	}
	chave, err := tenants.CriarChaveComNome(ctx, a.tenantA, "teste")
	if err != nil {
		t.Fatal(err)
	}
	a.chaveA = chave.Segredo
	a.h = httpapi.Novo(httpapi.Deps{
		Pool: b.App, Tenants: tenants,
		Auth:      painelauth.Novo(b.App, tenants, painelauth.Opcoes{ChaveCifra: chaveCifra, CookieInseguro: true}),
		Consultas: consulta.Novo(b.App, bureau.NovoSimulado(0), chaveHMAC, chaveCifra),
	})
	return a
}

func hash(s string) []byte { h := sha256.Sum256([]byte(s)); return h[:] }

func (a *ambiente) dono(sql string, args ...any) {
	a.t.Helper()
	if _, err := a.banco.Dono.Exec(context.Background(), sql, args...); err != nil {
		a.t.Fatalf("%s: %v", sql, err)
	}
}

func (a *ambiente) contar(sql string, args ...any) int {
	a.t.Helper()
	var n int
	if err := a.banco.Dono.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		a.t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// api assina e chama POST /v1/consultas como o cliente faria.
func (a *ambiente) api(corpo, idem string, assinaturaValida bool) *httptest.ResponseRecorder {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	ass := tenant.Assinar(a.chaveA, ts, []byte(corpo))
	if !assinaturaValida {
		ass = tenant.Assinar(a.chaveA, ts, []byte(corpo+" "))
	}
	r := httptest.NewRequest(http.MethodPost, "/v1/consultas", strings.NewReader(corpo))
	r.Header.Set("Authorization", "Bearer "+a.chaveA)
	r.Header.Set("X-Timestamp", ts)
	r.Header.Set("X-Assinatura", ass)
	if idem != "" {
		r.Header.Set("Idempotency-Key", idem)
	}
	w := httptest.NewRecorder()
	a.h.ServeHTTP(w, r)
	return w
}

func (a *ambiente) painel(s sessao, metodo, caminho, corpo string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(metodo, caminho, strings.NewReader(corpo))
	r.AddCookie(&http.Cookie{Name: painelauth.CookieSessao, Value: s.token})
	r.AddCookie(&http.Cookie{Name: painelauth.CookieCSRF, Value: s.csrf})
	r.Header.Set(painelauth.CabecalhoCSRF, s.csrf)
	w := httptest.NewRecorder()
	a.h.ServeHTTP(w, r)
	return w
}

func corpoConsulta(doc string, valor int64) string {
	return `{"documento":"` + doc + `","finalidade":"concessao_credito","pedido":{"valor":` +
		strconv.FormatInt(valor, 10) + `,"prazo":24},"dados":{"nome":"Fulano","email":"fulano@exemplo.com.br"}}`
}

func decodificar[T any](t *testing.T, w *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d, quer %d: %s", w.Code, status, w.Body.String())
	}
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("json: %v: %s", err, w.Body.String())
	}
	return v
}

func motivo(r consulta.Resposta) string {
	if len(r.Motivos) == 0 {
		return ""
	}
	return r.Motivos[len(r.Motivos)-1].Codigo
}

func TestIntegracao(t *testing.T) {
	a := novoAmbiente(t)
	t.Run("saude e cabeçalhos de segurança", a.testarSaude)
	t.Run("consultas fim a fim", a.testarConsultas)
	t.Run("idempotência", a.testarIdempotencia)
	t.Run("isolamento entre tenants", a.testarIsolamento)
	t.Run("ciclo da política", a.testarPolitica)
	t.Run("visão geral", a.testarVisaoGeral)
	t.Run("auditoria encadeada", a.testarAuditoria) // por último: adultera a cadeia
}

func (a *ambiente) testarSaude(t *testing.T) {
	w := httptest.NewRecorder()
	a.h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/saude", nil))
	if w.Code != http.StatusOK || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("saude: %d %v", w.Code, w.Header())
	}
	w = a.painel(a.sessA, http.MethodPut, "/painel/politicas/rascunho", `{"x":"`+strings.Repeat("a", httpapi.MaxCorpo)+`"}`)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("corpo acima de 64 KB: %d %s", w.Code, w.Body.String())
	}
}

func (a *ambiente) testarConsultas(t *testing.T) {
	casos := []struct {
		nome, doc string
		valor     int64
		status    string
		motivo    string
	}{
		{"aprovado", bureau.CPFScoreAlto, 1_000_000, "aprovado", ""},
		{"negado por corte", bureau.CPFScoreBaixo, 100_000, "negado", "RESTRICOES"},
		{"contraproposta", bureau.CPFScoreAlto, 5_000_000, "contraproposta", politica.MotivoLimiteFaixa},
		{"documento inválido", "123.456.789-00", 100_000, "documento_invalido", consulta.MotivoDocumentoInvalido},
		{"falecido", bureau.CPFFalecido, 100_000, "documento_invalido", consulta.MotivoDocumentoInvalido},
		{"bureau indisponível", bureau.CPFIndisponivel, 100_000, "negado", consulta.MotivoBureauIndisponivel},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			r := decodificar[consulta.Resposta](t, a.api(corpoConsulta(c.doc, c.valor), "", true), http.StatusOK)
			if r.Status != c.status || (c.motivo != "" && motivo(r) != c.motivo) {
				t.Fatalf("decisão %s %v; quer %s %s", r.Status, r.Motivos, c.status, c.motivo)
			}
			if c.status == "aprovado" && (r.Politica == nil || r.Politica.Versao != 1 || r.FaixaRisco == nil || *r.FaixaRisco != "A") {
				t.Fatalf("aprovado sem política/faixa certas: %+v", r)
			}
			if c.nome == "contraproposta" && (r.Oferta == nil || r.Oferta.Valor != 3_000_000) {
				t.Fatalf("oferta = %+v, quer o limite fixo da faixa A", r.Oferta)
			}
		})
	}

	t.Run("documento nunca em claro no banco", func(t *testing.T) {
		r := decodificar[consulta.Resposta](t, a.api(corpoConsulta("000.000.004-34", 1_000_000), "", true), http.StatusOK)
		var linha string
		var docHash, cifrado []byte
		var versao int
		err := a.banco.Dono.QueryRow(context.Background(), `SELECT row_to_json(c)::text, doc_hash, doc_cifrado, politica_versao
			FROM consultas c WHERE id = $1`, r.ID).Scan(&linha, &docHash, &cifrado, &versao)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(linha, bureau.CPFScoreAlto) || strings.Contains(linha, "Fulano") || strings.Contains(linha, "fulano@") {
			t.Fatalf("PII em claro na linha: %s", linha)
		}
		claro, err := plataforma.Decifrar(chaveCifra, cifrado)
		if err != nil || string(claro) != bureau.CPFScoreAlto || !bytes.Equal(docHash, plataforma.HashDoc(chaveHMAC, bureau.CPFScoreAlto)) || versao != 1 {
			t.Fatalf("doc cifrado/hash/versão errados: %q %v %d", claro, err, versao)
		}
	})

	t.Run("bureau lento nega em ~2s", func(t *testing.T) {
		inicio := time.Now()
		r := decodificar[consulta.Resposta](t, a.api(corpoConsulta(bureau.CPFLento, 100_000), "", true), http.StatusOK)
		decorrido := time.Since(inicio)
		if r.Status != "negado" || motivo(r) != consulta.MotivoBureauIndisponivel {
			t.Fatalf("decisão %s %v", r.Status, r.Motivos)
		}
		if decorrido < consulta.TempoMaximoFontes || decorrido > consulta.TempoMaximoFontes+time.Second {
			t.Fatalf("levou %v; quer ~%v", decorrido, consulta.TempoMaximoFontes)
		}
	})

	t.Run("assinatura inválida", func(t *testing.T) {
		antes := a.contar(`SELECT count(*) FROM consultas`)
		if w := a.api(corpoConsulta(bureau.CPFScoreAlto, 100_000), "", false); w.Code != http.StatusUnauthorized ||
			!strings.Contains(w.Body.String(), "nao_autorizado") {
			t.Fatalf("assinatura inválida: %d %s", w.Code, w.Body.String())
		}
		if depois := a.contar(`SELECT count(*) FROM consultas`); depois != antes {
			t.Fatalf("consulta gravada sem autenticação")
		}
	})

	t.Run("corpo inválido", func(t *testing.T) {
		e := decodificar[map[string]any](t, a.api(`{"documento":"1","finalidade":"x","pedido":{"valor":0,"prazo":24}}`, "", true), http.StatusBadRequest)
		campos, _ := e["campos"].(map[string]any)
		if e["erro"] != "corpo_invalido" || campos["pedido.valor"] == nil || campos["finalidade"] == nil {
			t.Fatalf("erro = %v", e)
		}
	})
}

func (a *ambiente) testarIdempotencia(t *testing.T) {
	corpo := corpoConsulta(bureau.CPFScoreAlto, 200_000)
	w1 := a.api(corpo, "pedido-123", true)
	w2 := a.api(corpo, "pedido-123", true)
	if w1.Code != http.StatusOK || w2.Code != http.StatusOK || w1.Body.String() != w2.Body.String() {
		t.Fatalf("reenvio diferente:\n%d %s\n%d %s", w1.Code, w1.Body.String(), w2.Code, w2.Body.String())
	}
	if n := a.contar(`SELECT count(*) FROM consultas WHERE idempotency_key = 'pedido-123'`); n != 1 {
		t.Fatalf("%d linhas para a mesma Idempotency-Key", n)
	}
	w3 := a.api(corpoConsulta(bureau.CPFScoreAlto, 300_000), "pedido-123", true)
	if w3.Code != http.StatusConflict || !strings.Contains(w3.Body.String(), "conflito_idempotencia") {
		t.Fatalf("corpo diferente: %d %s", w3.Code, w3.Body.String())
	}
}

func (a *ambiente) testarIsolamento(t *testing.T) {
	r := decodificar[consulta.Resposta](t, a.api(corpoConsulta(bureau.CPFScoreAlto, 100_000), "", true), http.StatusOK)

	d := decodificar[consulta.Detalhe](t, a.painel(a.sessA, http.MethodGet, "/painel/consultas/"+r.ID, ""), http.StatusOK)
	if d.DocumentoMascarado != "***.000.004-**" || len(d.Trilha) == 0 || d.Politica == nil || d.Finalidade != "concessao_credito" ||
		strings.Join(d.DadosInformados, ",") != "nome,email" {
		t.Fatalf("detalhe = %+v", d)
	}
	if w := a.painel(a.sessB, http.MethodGet, "/painel/consultas/"+r.ID, ""); w.Code != http.StatusNotFound {
		t.Fatalf("B leu consulta de A: %d %s", w.Code, w.Body.String())
	}
	listaB := decodificar[struct {
		Itens []consulta.Resumo `json:"itens"`
	}](t, a.painel(a.sessB, http.MethodGet, "/painel/consultas", ""), http.StatusOK)
	if len(listaB.Itens) != 0 {
		t.Fatalf("B vê %d consultas de A", len(listaB.Itens))
	}

	listaA := decodificar[struct {
		Itens   []consulta.Resumo `json:"itens"`
		Proximo *string           `json:"proximo_cursor"`
	}](t, a.painel(a.sessA, http.MethodGet, "/painel/consultas?documento=000.000.004-34&limite=2", ""), http.StatusOK)
	if len(listaA.Itens) != 2 || listaA.Proximo == nil || listaA.Itens[0].ID != r.ID {
		t.Fatalf("lista de A por documento: %+v", listaA)
	}
	pag2 := decodificar[struct {
		Itens []consulta.Resumo `json:"itens"`
	}](t,
		a.painel(a.sessA, http.MethodGet, "/painel/consultas?documento=000.000.004-34&limite=2&cursor="+*listaA.Proximo, ""), http.StatusOK)
	if len(pag2.Itens) == 0 || pag2.Itens[0].ID >= *listaA.Proximo {
		t.Fatalf("segunda página: %+v", pag2)
	}
	if w := a.painel(a.sessA, http.MethodGet, "/painel/consultas?status=xyz", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("filtro inválido: %d", w.Code)
	}
}

func (a *ambiente) testarPolitica(t *testing.T) {
	invalida := `{"cortes":[],"faixas":[{"rotulo":"A","cel":"renda > 100","nega":false,"taxa_bps":100,"limite_fixo":1000,"multiplo_renda_x10":0}],
		"contraproposta":{"ativa":false,"prazo_maximo":0,"acrescimo_taxa_bps":0},"expressao_avancada":true}`
	if w := a.painel(a.sessA, http.MethodPut, "/painel/politicas/rascunho", invalida); w.Code != http.StatusOK {
		t.Fatalf("salvar rascunho: %d %s", w.Code, w.Body.String())
	}
	e := decodificar[struct {
		Erro     string              `json:"erro"`
		Detalhes []politica.Problema `json:"detalhes"`
	}](t, a.painel(a.sessA, http.MethodPost, "/painel/politicas/publicar", `{"confirmacao":true}`), http.StatusUnprocessableEntity)
	if e.Erro != "politica_invalida" || len(e.Detalhes) == 0 || !strings.HasPrefix(e.Detalhes[0].Regra, "faixas[0]") {
		t.Fatalf("422 sem a regra: %+v", e)
	}
	if w := a.painel(a.sessA, http.MethodPost, "/painel/politicas/publicar", `{"confirmacao":false}`); w.Code != http.StatusBadRequest {
		t.Fatalf("publicar sem confirmação: %d", w.Code)
	}

	nova := politica.ModeloEmprestimoPF()
	nova.Faixas[2].TaxaBps = 250
	bruto, _ := json.Marshal(nova)
	if w := a.painel(a.sessA, http.MethodPut, "/painel/politicas/rascunho", string(bruto)); w.Code != http.StatusOK {
		t.Fatalf("salvar rascunho: %d %s", w.Code, w.Body.String())
	}

	antes := a.contar(`SELECT count(*) FROM consultas`)
	teste := decodificar[consulta.ResultadoTeste](t, a.painel(a.sessA, http.MethodPost, "/painel/politicas/testar",
		`{"documento":"000.000.004-34","pedido":{"valor":100000,"prazo":12}}`), http.StatusOK)
	if teste.Status != "aprovado" || teste.Oferta == nil || teste.Oferta.TaxaBps != 250 || len(teste.Trilha) == 0 {
		t.Fatalf("teste do rascunho: %+v", teste)
	}
	if a.contar(`SELECT count(*) FROM consultas`) != antes {
		t.Fatal("testar a política gravou consulta")
	}

	v2 := decodificar[repositorio.Versao](t, a.painel(a.sessA, http.MethodPost, "/painel/politicas/publicar", `{"confirmacao":true,"comentario":"taxa A 2,50%"}`), http.StatusCreated)
	if v2.Versao != 2 || v2.Conteudo == nil || v2.Conteudo.Faixas[2].TaxaBps != 250 {
		t.Fatalf("versão publicada: %+v", v2)
	}
	a.quererVersaoNaDecisao(t, 2, 250)

	dif := decodificar[struct {
		Mudancas []httpapi.Mudanca `json:"mudancas"`
	}](t, a.painel(a.sessA, http.MethodGet, "/painel/politicas/diferenca?de=1&para=2", ""), http.StatusOK)
	if len(dif.Mudancas) != 1 || dif.Mudancas[0].Caminho != "faixas[2].taxa_bps" || dif.Mudancas[0].Tipo != "alterado" {
		t.Fatalf("diferença: %+v", dif.Mudancas)
	}

	v3 := decodificar[repositorio.Versao](t, a.painel(a.sessA, http.MethodPost, "/painel/politicas/versoes/1/reverter", `{"confirmacao":true}`), http.StatusCreated)
	if v3.Versao != 3 || v3.RevertidaDe == nil || *v3.RevertidaDe != 1 {
		t.Fatalf("reversão: %+v", v3)
	}
	a.quererVersaoNaDecisao(t, 3, 290)

	versoes := decodificar[map[string][]repositorio.Versao](t, a.painel(a.sessA, http.MethodGet, "/painel/politicas/versoes", ""), http.StatusOK)
	if len(versoes["itens"]) != 3 || versoes["itens"][0].Versao != 3 || versoes["itens"][0].Conteudo != nil {
		t.Fatalf("histórico: %+v", versoes)
	}
	if w := a.painel(a.sessB, http.MethodGet, "/painel/politicas/versoes/3", ""); w.Code != http.StatusNotFound {
		t.Fatalf("B leu versão de A: %d", w.Code)
	}
	if w := a.painel(a.sessB, http.MethodPost, "/painel/politicas/versoes/3/reverter", `{"confirmacao":true}`); w.Code != http.StatusNotFound {
		t.Fatalf("B reverteu para versão de A: %d", w.Code)
	}
}

// quererVersaoNaDecisao confere a versão na resposta e na linha gravada.
func (a *ambiente) quererVersaoNaDecisao(t *testing.T, versao, taxa int) {
	t.Helper()
	r := decodificar[consulta.Resposta](t, a.api(corpoConsulta(bureau.CPFScoreAlto, 100_000), "", true), http.StatusOK)
	if r.Politica == nil || r.Politica.Versao != versao || r.Oferta == nil || r.Oferta.TaxaBps != taxa {
		t.Fatalf("decisão com política %+v oferta %+v; quer versão %d taxa %d", r.Politica, r.Oferta, versao, taxa)
	}
	if n := a.contar(`SELECT politica_versao FROM consultas WHERE id = $1`, r.ID); n != versao {
		t.Fatalf("gravou versão %d, quer %d", n, versao)
	}
}

func (a *ambiente) testarVisaoGeral(t *testing.T) {
	v := decodificar[repositorio.VisaoGeral](t, a.painel(a.sessA, http.MethodGet, "/painel/visao-geral?periodo=7d", ""), http.StatusOK)
	soma := 0
	for _, f := range v.DistribuicaoScore {
		soma += f.Quantidade
	}
	if v.Total == 0 || v.PorStatus["aprovado"] == 0 || v.TaxaAprovacaoBps == 0 || len(v.DistribuicaoScore) != 10 || soma == 0 {
		t.Fatalf("visão geral: %+v", v)
	}
	vb := decodificar[repositorio.VisaoGeral](t, a.painel(a.sessB, http.MethodGet, "/painel/visao-geral", ""), http.StatusOK)
	if vb.Total != 0 {
		t.Fatalf("B vê números de A: %+v", vb)
	}
}

func (a *ambiente) testarAuditoria(t *testing.T) {
	ctx := context.Background()
	err := tenant.EmTenant(ctx, a.banco.App, a.tenantA, func(tx pgx.Tx) error { return auditoria.Verificar(ctx, tx, a.tenantA) })
	if err != nil {
		t.Fatalf("cadeia íntegra recusada: %v", err)
	}
	if n := a.contar(`SELECT count(*) FROM auditoria WHERE tenant_id = $1 AND evento = $2`, a.tenantA, auditoria.EventoPoliticaPublicada); n != 3 {
		t.Fatalf("%d publicações auditadas, quer 3", n)
	}
	if n := a.contar(`SELECT count(*) FROM auditoria a JOIN consultas c ON c.id = a.consulta_id
		WHERE a.tenant_id = $1 AND a.ator LIKE 'api:%'`, a.tenantA); n != a.contar(`SELECT count(*) FROM consultas WHERE tenant_id = $1`, a.tenantA) {
		t.Fatal("consulta sem auditoria (ou auditoria sem consulta)")
	}
	if _, err := a.banco.App.Exec(ctx, `UPDATE auditoria SET dados = '{}'`); err == nil {
		t.Fatal("motor_app conseguiu alterar a auditoria")
	}

	a.dono(`UPDATE auditoria SET dados = jsonb_set(dados, '{status}', '"aprovado"') WHERE tenant_id = $1 AND seq = 3`, a.tenantA)
	err = auditoria.Verificar(ctx, a.banco.Dono, a.tenantA)
	if !errors.Is(err, auditoria.ErrCadeiaQuebrada) || !strings.Contains(err.Error(), "seq 3") {
		t.Fatalf("adulteração não detectada: %v", err)
	}
	if err := auditoria.Verificar(ctx, a.banco.Dono, a.tenantB); err != nil {
		t.Fatalf("cadeia de B afetada: %v", err)
	}
}
