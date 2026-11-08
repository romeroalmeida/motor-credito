package painelauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pquerna/otp/totp"

	"github.com/romeroalmeida/motor-credito/internal/plataforma"
	"github.com/romeroalmeida/motor-credito/internal/plataforma/bancoteste"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

var chaveCifraTeste = bytes.Repeat([]byte{9}, 32)

const senhaTeste = "senha-bem-comprida-1"

type ambiente struct {
	t       *testing.T
	b       *bancoteste.Banco
	rotas   http.Handler
	relogio time.Time
	proxIP  int
}

type usuarioTeste struct{ id, email, segredo string }

// Um container só para o pacote: os casos rodam como subtestes.
func TestIntegracao(t *testing.T) {
	a := &ambiente{t: t, b: bancoteste.Novo(t), relogio: time.Now()}
	h := Novo(a.b.App, tenant.NovoServico(a.b.App, chaveCifraTeste), Opcoes{ChaveCifra: chaveCifraTeste, URLPainel: "https://painel.teste"})
	h.agora = func() time.Time { return a.relogio }
	r := chi.NewRouter()
	h.Rotas(r)
	a.rotas = r
	for _, id := range []string{"ten_a", "ten_b"} {
		a.exec(`INSERT INTO tenants (id, nome) VALUES ($1, $1)`, id)
	}

	t.Run("login completo e cookies", a.testarLoginCompleto)
	t.Run("senha errada e e-mail inexistente respondem igual", a.testarSenhaErrada)
	t.Run("totp errado, replay e limite", a.testarTOTP)
	t.Run("login com limite por e-mail+IP", a.testarLimiteLogin)
	t.Run("csrf", a.testarCSRF)
	t.Run("papel", a.testarPapel)
	t.Run("isolamento de tenant 404", a.testarIsolamento)
	t.Run("chaves e IPs", a.testarChavesEIPs)
	t.Run("convite e cadastro", a.testarConviteECadastro)
	t.Run("sair", a.testarSair)
}

func (a *ambiente) exec(sql string, args ...any) {
	a.t.Helper()
	if _, err := a.b.Dono.Exec(context.Background(), sql, args...); err != nil {
		a.t.Fatalf("%s: %v", sql, err)
	}
}

func (a *ambiente) semear(t *testing.T, tenantID, email string, papel Papel) usuarioTeste {
	t.Helper()
	hash, err := hashSenha(senhaTeste)
	if err != nil {
		t.Fatal(err)
	}
	segredo, _, err := novoSegredoTOTP(email)
	if err != nil {
		t.Fatal(err)
	}
	id := plataforma.NovoID("usr")
	cifrado, err := CifrarTOTP(chaveCifraTeste, id, segredo)
	if err != nil {
		t.Fatal(err)
	}
	a.exec(`INSERT INTO usuarios (id, email, nome, senha_hash) VALUES ($1, $2, $2, $3)`, id, email, hash)
	a.exec(`INSERT INTO totp (usuario_id, segredo_cifrado, confirmado_em) VALUES ($1, $2, now())`, id, cifrado)
	a.exec(`INSERT INTO associacoes (usuario_id, tenant_id, papel) VALUES ($1, $2, $3)`, id, tenantID, string(papel))
	return usuarioTeste{id: id, email: email, segredo: segredo}
}

// codigo avança o relógio um passo TOTP (anti-replay exige passo novo) e gera o código.
func (a *ambiente) codigo(segredo string) string {
	a.relogio = a.relogio.Add(30 * time.Second)
	return a.codigoAgora(segredo)
}

func (a *ambiente) codigoAgora(segredo string) string {
	c, err := totp.GenerateCode(segredo, a.relogio)
	if err != nil {
		a.t.Fatal(err)
	}
	return c
}

type cliente struct {
	a                *ambiente
	sessao, csrf, ip string
	ultimos          []*http.Cookie
}

func (a *ambiente) novoCliente() *cliente {
	a.proxIP++
	return &cliente{a: a, ip: fmt.Sprintf("10.0.%d.%d:4000", a.proxIP/250, a.proxIP%250+1)}
}

// fazer manda a requisição com os cookies do cliente e guarda os que a resposta definir.
func (c *cliente) fazer(metodo, caminho, corpo string, comCSRF bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(metodo, caminho, strings.NewReader(corpo))
	r.RemoteAddr = c.ip
	r.Header.Set("Content-Type", "application/json")
	if c.sessao != "" {
		r.AddCookie(&http.Cookie{Name: CookieSessao, Value: c.sessao})
	}
	if c.csrf != "" {
		r.AddCookie(&http.Cookie{Name: CookieCSRF, Value: c.csrf})
	}
	if comCSRF {
		r.Header.Set(CabecalhoCSRF, c.csrf)
	}
	w := httptest.NewRecorder()
	c.a.rotas.ServeHTTP(w, r)
	c.ultimos = w.Result().Cookies()
	for _, ck := range c.ultimos {
		switch ck.Name {
		case CookieSessao:
			c.sessao = ck.Value
		case CookieCSRF:
			c.csrf = ck.Value
		}
	}
	return w
}

func (a *ambiente) entrar(t *testing.T, u usuarioTeste) *cliente {
	t.Helper()
	c := a.novoCliente()
	if w := c.fazer("POST", "/painel/auth/entrar", `{"email":"`+u.email+`","senha":"`+senhaTeste+`"}`, false); w.Code != 200 {
		t.Fatalf("entrar: %d %s", w.Code, w.Body)
	}
	if w := c.fazer("POST", "/painel/auth/2fa", `{"codigo":"`+a.codigo(u.segredo)+`"}`, true); w.Code != 200 {
		t.Fatalf("2fa: %d %s", w.Code, w.Body)
	}
	return c
}

func quer(t *testing.T, w *httptest.ResponseRecorder, status int, erro string) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status = %d; quer %d (%s)", w.Code, status, w.Body)
	}
	if erro != "" && !strings.Contains(w.Body.String(), `"erro":"`+erro+`"`) {
		t.Fatalf("corpo = %s; quer erro %q", w.Body, erro)
	}
}

func (a *ambiente) testarLoginCompleto(t *testing.T) {
	u := a.semear(t, "ten_a", "Admin.Login@exemplo.com", PapelAdmin)
	c := a.novoCliente()
	w := c.fazer("POST", "/painel/auth/entrar", `{"email":"admin.login@exemplo.com ","senha":"`+senhaTeste+`"}`, false)
	quer(t, w, 200, "")
	if strings.TrimSpace(w.Body.String()) != `{"proximo":"2fa"}` {
		t.Fatalf("corpo = %s", w.Body)
	}
	for _, ck := range c.ultimos {
		if !ck.Secure || ck.SameSite != http.SameSiteStrictMode || ck.HttpOnly != (ck.Name == CookieSessao) {
			t.Fatalf("cookie %s: Secure=%v SameSite=%v HttpOnly=%v", ck.Name, ck.Secure, ck.SameSite, ck.HttpOnly)
		}
	}
	pendente, pendenteCSRF := c.sessao, c.csrf

	quer(t, c.fazer("GET", "/painel/auth/eu", "", false), 401, "nao_autorizado") // sessão pendente não entra
	quer(t, c.fazer("POST", "/painel/auth/2fa", `{"codigo":"`+a.codigo(u.segredo)+`"}`, false), 403, "csrf_invalido")

	w = c.fazer("POST", "/painel/auth/2fa", `{"codigo":"`+a.codigo(u.segredo)+`"}`, true)
	quer(t, w, 200, "")
	var eu respostaEu
	if err := json.Unmarshal(w.Body.Bytes(), &eu); err != nil || eu.Papel != PapelAdmin || eu.Tenant.ID != "ten_a" || eu.Usuario.ID != u.id {
		t.Fatalf("eu = %+v (%v)", eu, err)
	}
	if c.sessao == pendente {
		t.Fatal("o cookie de sessão precisa girar depois do TOTP")
	}
	quer(t, c.fazer("GET", "/painel/auth/eu", "", false), 200, "")

	velho := &cliente{a: a, ip: c.ip, sessao: pendente, csrf: pendenteCSRF}
	quer(t, velho.fazer("GET", "/painel/auth/eu", "", false), 401, "nao_autorizado")
	// A sessão pendente foi encerrada: não serve para um segundo TOTP (que criaria outra sessão completa).
	quer(t, velho.fazer("POST", "/painel/auth/2fa", `{"codigo":"`+a.codigo(u.segredo)+`"}`, true), 401, "nao_autorizado")
	quer(t, a.novoCliente().fazer("GET", "/painel/auth/eu", "", false), 401, "nao_autorizado")
	quer(t, a.novoCliente().fazer("GET", "/painel/chaves", "", false), 401, "nao_autorizado")
}

func (a *ambiente) testarSenhaErrada(t *testing.T) {
	a.semear(t, "ten_a", "senha.errada@exemplo.com", PapelAdmin)
	errada := a.novoCliente().fazer("POST", "/painel/auth/entrar", `{"email":"senha.errada@exemplo.com","senha":"outra-senha-qualquer"}`, false)
	inexistente := a.novoCliente().fazer("POST", "/painel/auth/entrar", `{"email":"ninguem@exemplo.com","senha":"outra-senha-qualquer"}`, false)
	quer(t, errada, 401, "nao_autorizado")
	if errada.Body.String() != inexistente.Body.String() || errada.Code != inexistente.Code {
		t.Fatalf("respostas diferentes: %d %s / %d %s", errada.Code, errada.Body, inexistente.Code, inexistente.Body)
	}
	if len(errada.Result().Cookies()) != 0 {
		t.Fatal("senha errada não pode definir cookie")
	}
}

func (a *ambiente) testarTOTP(t *testing.T) {
	u := a.semear(t, "ten_a", "totp@exemplo.com", PapelAnalista)
	c := a.novoCliente()
	quer(t, c.fazer("POST", "/painel/auth/entrar", `{"email":"totp@exemplo.com","senha":"`+senhaTeste+`"}`, false), 200, "")
	certo := a.codigo(u.segredo)
	errado := fmt.Sprintf("%06d", (atoi(certo)+1)%1_000_000)
	quer(t, c.fazer("POST", "/painel/auth/2fa", `{"codigo":"`+errado+`"}`, true), 401, "nao_autorizado")
	quer(t, c.fazer("POST", "/painel/auth/2fa", `{"codigo":"abc"}`, true), 401, "nao_autorizado")
	quer(t, c.fazer("POST", "/painel/auth/2fa", `{"codigo":"`+certo+`"}`, true), 200, "")

	// Replay: o mesmo código (mesmo passo) numa sessão nova não vale.
	c2 := a.novoCliente()
	quer(t, c2.fazer("POST", "/painel/auth/entrar", `{"email":"totp@exemplo.com","senha":"`+senhaTeste+`"}`, false), 200, "")
	quer(t, c2.fazer("POST", "/painel/auth/2fa", `{"codigo":"`+a.codigoAgora(u.segredo)+`"}`, true), 401, "nao_autorizado")

	// Limite de 5 códigos por usuário na janela (4 já usados acima): o 6º é 429 mesmo certo.
	quer(t, c2.fazer("POST", "/painel/auth/2fa", `{"codigo":"`+errado+`"}`, true), 401, "nao_autorizado")
	w := c2.fazer("POST", "/painel/auth/2fa", `{"codigo":"`+a.codigo(u.segredo)+`"}`, true)
	quer(t, w, 429, "limite_excedido")
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("429 sem Retry-After")
	}
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

func (a *ambiente) testarLimiteLogin(t *testing.T) {
	a.semear(t, "ten_a", "limite@exemplo.com", PapelAdmin)
	c := a.novoCliente()
	corpo := `{"email":"limite@exemplo.com","senha":"errada-errada"}`
	for i := 0; i < limiteEntrar; i++ {
		quer(t, c.fazer("POST", "/painel/auth/entrar", corpo, false), 401, "nao_autorizado")
	}
	certo := `{"email":"limite@exemplo.com","senha":"` + senhaTeste + `"}`
	w := c.fazer("POST", "/painel/auth/entrar", certo, false)
	quer(t, w, 429, "limite_excedido")
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("429 sem Retry-After")
	}
	quer(t, a.novoCliente().fazer("POST", "/painel/auth/entrar", certo, false), 200, "") // outro IP: chave é e-mail+IP
}

func (a *ambiente) testarCSRF(t *testing.T) {
	u := a.semear(t, "ten_a", "csrf@exemplo.com", PapelAdmin)
	c := a.entrar(t, u)
	quer(t, c.fazer("POST", "/painel/chaves", `{}`, false), 403, "csrf_invalido")

	errado := *c
	errado.csrf = "token-inventado"
	quer(t, errado.fazer("POST", "/painel/chaves", `{}`, true), 403, "csrf_invalido") // cookie == cabeçalho, mas não é desta sessão

	outra := a.entrar(t, u)
	misturado := &cliente{a: a, ip: c.ip, sessao: c.sessao, csrf: outra.csrf}
	quer(t, misturado.fazer("DELETE", "/painel/chaves/qualquer", ``, true), 403, "csrf_invalido") // CSRF de outra sessão

	quer(t, c.fazer("POST", "/painel/auth/sair", ``, false), 403, "csrf_invalido") // sair sem CSRF não derruba
	quer(t, c.fazer("GET", "/painel/auth/eu", "", false), 200, "")
	quer(t, c.fazer("POST", "/painel/chaves", `{"nome":"ok"}`, true), 201, "")
}

func (a *ambiente) testarPapel(t *testing.T) {
	analista := a.entrar(t, a.semear(t, "ten_a", "analista@exemplo.com", PapelAnalista))
	quer(t, analista.fazer("GET", "/painel/auth/eu", "", false), 200, "")
	quer(t, analista.fazer("GET", "/painel/chaves", "", false), 403, "proibido")
	quer(t, analista.fazer("POST", "/painel/equipe", `{"email":"x@exemplo.com","papel":"admin"}`, true), 403, "proibido")
	quer(t, analista.fazer("GET", "/painel/equipe", "", false), 403, "proibido")

	// Papel é relido do banco: rebaixar vale no request seguinte, sem novo login.
	u := a.semear(t, "ten_a", "rebaixado@exemplo.com", PapelAdmin)
	admin := a.entrar(t, u)
	quer(t, admin.fazer("GET", "/painel/chaves", "", false), 200, "")
	a.exec(`UPDATE associacoes SET papel = 'auditor' WHERE usuario_id = $1`, u.id)
	quer(t, admin.fazer("GET", "/painel/chaves", "", false), 403, "proibido")
}

func (a *ambiente) testarIsolamento(t *testing.T) {
	adminA := a.entrar(t, a.semear(t, "ten_a", "iso.a@exemplo.com", PapelAdmin))
	adminB := a.entrar(t, a.semear(t, "ten_b", "iso.b@exemplo.com", PapelAdmin))
	w := adminA.fazer("POST", "/painel/chaves", `{"nome":"do A"}`, true)
	quer(t, w, 201, "")
	var criada chaveCriadaJSON
	_ = json.Unmarshal(w.Body.Bytes(), &criada)

	quer(t, adminB.fazer("DELETE", "/painel/chaves/"+criada.ID, ``, true), 404, "nao_encontrado")
	quer(t, adminB.fazer("POST", "/painel/chaves/"+criada.ID+"/girar", ``, true), 404, "nao_encontrado")
	if lista := adminB.fazer("GET", "/painel/chaves", "", false).Body.String(); strings.Contains(lista, criada.ID) {
		t.Fatal("tenant B vê chave do A")
	}
	if equipe := adminB.fazer("GET", "/painel/equipe", "", false).Body.String(); strings.Contains(equipe, "iso.a@") {
		t.Fatal("tenant B vê membro do A")
	}
	lista := adminA.fazer("GET", "/painel/chaves", "", false).Body.String()
	if !strings.Contains(lista, criada.ID) || strings.Contains(lista, `"revogada_em":"`) {
		t.Fatalf("chave do A sumiu ou foi revogada: %s", lista)
	}
}

func (a *ambiente) testarChavesEIPs(t *testing.T) {
	c := a.entrar(t, a.semear(t, "ten_b", "chaves@exemplo.com", PapelAdmin))
	w := c.fazer("POST", "/painel/chaves", ``, true) // corpo opcional
	quer(t, w, 201, "")
	var criada chaveCriadaJSON
	_ = json.Unmarshal(w.Body.Bytes(), &criada)
	if criada.Chave != "ck_live_"+criada.ID || len(criada.Segredo) != 64 {
		t.Fatalf("chave criada = %+v", criada)
	}
	lista := c.fazer("GET", "/painel/chaves", "", false).Body.String()
	if strings.Contains(lista, criada.Segredo) || !strings.Contains(lista, `"prefixo":"ck_live_`+criada.ID+`"`) {
		t.Fatalf("lista expõe segredo ou sem prefixo: %s", lista)
	}
	quer(t, c.fazer("POST", "/painel/chaves", `{"nome":"`+strings.Repeat("n", maxNomeChave+1)+`"}`, true), 400, "corpo_invalido")

	w = c.fazer("POST", "/painel/chaves/"+criada.ID+"/girar", ``, true)
	quer(t, w, 201, "")
	quer(t, c.fazer("DELETE", "/painel/chaves/"+criada.ID, ``, true), 204, "")
	quer(t, c.fazer("DELETE", "/painel/chaves/inexistente", ``, true), 404, "nao_encontrado")

	quer(t, c.fazer("GET", "/painel/chaves/ips", "", false), 200, "")
	w = c.fazer("PUT", "/painel/chaves/ips", `{"ips":["200.150.10.7/24","2001:db8::1"]}`, true)
	quer(t, w, 200, "")
	if strings.TrimSpace(w.Body.String()) != `{"ips":["200.150.10.0/24","2001:db8::1/128"]}` {
		t.Fatalf("ips = %s", w.Body)
	}
	quer(t, c.fazer("PUT", "/painel/chaves/ips", `{"ips":["nao-e-ip"]}`, true), 400, "corpo_invalido")
	quer(t, c.fazer("PUT", "/painel/chaves/ips", `{}`, true), 400, "corpo_invalido")
	if body := c.fazer("GET", "/painel/chaves/ips", "", false).Body.String(); !strings.Contains(body, "200.150.10.0/24") {
		t.Fatalf("ips não persistiram: %s", body)
	}
}

func (a *ambiente) testarConviteECadastro(t *testing.T) {
	admin := a.entrar(t, a.semear(t, "ten_a", "convida@exemplo.com", PapelAdmin))
	quer(t, admin.fazer("POST", "/painel/equipe", `{"email":"nao-e-email","papel":"admin"}`, true), 400, "corpo_invalido")
	quer(t, admin.fazer("POST", "/painel/equipe", `{"email":"novo@exemplo.com","papel":"chefe"}`, true), 400, "corpo_invalido")
	quer(t, admin.fazer("POST", "/painel/equipe", `{"email":"Convida@exemplo.com","papel":"analista"}`, true), 409, "conflito")

	w := admin.fazer("POST", "/painel/equipe", `{"email":"novo@exemplo.com","papel":"auditor"}`, true)
	quer(t, w, 201, "")
	var cv conviteJSON
	_ = json.Unmarshal(w.Body.Bytes(), &cv)
	if !strings.HasPrefix(cv.Link, "https://painel.teste/cadastro?token=") || cv.Papel != PapelAuditor {
		t.Fatalf("convite = %+v", cv)
	}
	quer(t, admin.fazer("POST", "/painel/equipe", `{"email":"NOVO@exemplo.com","papel":"auditor"}`, true), 409, "conflito")
	if equipe := admin.fazer("GET", "/painel/equipe", "", false).Body.String(); !strings.Contains(equipe, cv.ID) || strings.Contains(equipe, "token") {
		t.Fatalf("equipe = %s", equipe)
	}

	link, _ := url.Parse(cv.Link)
	token := link.Query().Get("token")
	novo := a.novoCliente()
	quer(t, novo.fazer("POST", "/painel/auth/cadastro", `{"token":"`+token+`","nome":"Nova","senha":"curta"}`, false), 400, "corpo_invalido")
	quer(t, novo.fazer("POST", "/painel/auth/cadastro", `{"token":"inventado","nome":"Nova","senha":"`+senhaTeste+`"}`, false), 404, "nao_encontrado")
	w = novo.fazer("POST", "/painel/auth/cadastro", `{"token":"`+token+`","nome":"Nova","senha":"`+senhaTeste+`"}`, false)
	quer(t, w, 200, "")
	var resp struct{ Proximo, Otpauth_url, Segredo_totp string }
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Proximo != "2fa" || !strings.HasPrefix(resp.Otpauth_url, "otpauth://totp/") || resp.Segredo_totp == "" {
		t.Fatalf("cadastro = %s", w.Body)
	}
	quer(t, novo.fazer("GET", "/painel/auth/eu", "", false), 401, "nao_autorizado") // falta o TOTP
	w = novo.fazer("POST", "/painel/auth/2fa", `{"codigo":"`+a.codigo(resp.Segredo_totp)+`"}`, true)
	quer(t, w, 200, "")
	if !strings.Contains(w.Body.String(), `"papel":"auditor"`) {
		t.Fatalf("eu = %s", w.Body)
	}
	quer(t, a.novoCliente().fazer("POST", "/painel/auth/cadastro", `{"token":"`+token+`","nome":"De novo","senha":"`+senhaTeste+`"}`, false), 404, "nao_encontrado")

	// Convite vencido → 404.
	w = admin.fazer("POST", "/painel/equipe", `{"email":"vencido@exemplo.com","papel":"analista"}`, true)
	quer(t, w, 201, "")
	_ = json.Unmarshal(w.Body.Bytes(), &cv)
	a.exec(`UPDATE convites SET expira_em = now() - interval '1 second' WHERE id = $1`, cv.ID)
	link, _ = url.Parse(cv.Link)
	quer(t, a.novoCliente().fazer("POST", "/painel/auth/cadastro", `{"token":"`+link.Query().Get("token")+`","nome":"V","senha":"`+senhaTeste+`"}`, false), 404, "nao_encontrado")
}

func (a *ambiente) testarSair(t *testing.T) {
	c := a.entrar(t, a.semear(t, "ten_a", "sair@exemplo.com", PapelAnalista))
	sessao := c.sessao
	w := c.fazer("POST", "/painel/auth/sair", ``, true)
	quer(t, w, 204, "")
	for _, ck := range w.Result().Cookies() {
		if ck.MaxAge >= 0 {
			t.Fatalf("cookie %s não foi apagado", ck.Name)
		}
	}
	revivida := &cliente{a: a, ip: c.ip, sessao: sessao}
	quer(t, revivida.fazer("GET", "/painel/auth/eu", "", false), 401, "nao_autorizado")
}
