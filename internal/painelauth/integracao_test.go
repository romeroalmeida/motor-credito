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
	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp/totp"

	"github.com/romeroalmeida/motor-credito/internal/auditoria"
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
	t.Run("trava persistente do TOTP", a.testarTravaTOTP)
	t.Run("sessão expira por inatividade", a.testarInatividade)
	t.Run("csrf", a.testarCSRF)
	t.Run("papel", a.testarPapel)
	t.Run("isolamento de tenant 404", a.testarIsolamento)
	t.Run("chaves e IPs", a.testarChavesEIPs)
	t.Run("convite e cadastro", a.testarConviteECadastro)
	t.Run("cadastro não revela e-mail com conta", a.testarCadastroSemEnumeracao)
	t.Run("sair", a.testarSair)
	t.Run("equipe: remover e trocar papel", a.testarRemoverETrocarPapel)
	t.Run("auditoria de chaves, equipe e auth", a.testarAuditoriaDeAcesso) // depois de todos: conta o que eles fizeram
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

	// Spray: o mesmo IP tentando e-mails diferentes esbarra no limite por IP, antes de qualquer argon2.
	spray := a.novoCliente()
	for i := 0; i < limiteEntrarIP; i++ {
		w := spray.fazer("POST", "/painel/auth/entrar", fmt.Sprintf(`{"email":"spray%d@exemplo.com","senha":"x"}`, i), false)
		quer(t, w, 401, "nao_autorizado")
	}
	quer(t, spray.fazer("POST", "/painel/auth/entrar", certo, false), 429, "limite_excedido")
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

// Cinco códigos errados travam o 2FA no banco: outra instância (limitador em memória zerado) também recusa,
// mesmo com o código certo, até a trava vencer (achado 6).
func (a *ambiente) testarTravaTOTP(t *testing.T) {
	u := a.semear(t, "ten_a", "trava@exemplo.com", PapelAnalista)
	c := a.novoCliente()
	quer(t, c.fazer("POST", "/painel/auth/entrar", `{"email":"trava@exemplo.com","senha":"`+senhaTeste+`"}`, false), 200, "")
	for range maxFalhasTOTP {
		quer(t, c.fazer("POST", "/painel/auth/2fa", `{"codigo":"000000"}`, true), 401, "nao_autorizado")
	}
	outra := Novo(a.b.App, tenant.NovoServico(a.b.App, chaveCifraTeste), Opcoes{ChaveCifra: chaveCifraTeste})
	outra.agora = func() time.Time { return a.relogio }
	r := chi.NewRouter()
	outra.Rotas(r)
	original := a.rotas
	a.rotas = r
	defer func() { a.rotas = original }()

	w := c.fazer("POST", "/painel/auth/2fa", `{"codigo":"`+a.codigo(u.segredo)+`"}`, true)
	quer(t, w, 429, "limite_excedido")
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("429 sem Retry-After")
	}
	a.exec(`UPDATE totp SET travado_ate = now() - interval '1 second' WHERE usuario_id = $1`, u.id)
	quer(t, c.fazer("POST", "/painel/auth/2fa", `{"codigo":"`+a.codigo(u.segredo)+`"}`, true), 200, "")
}

// Convite (em outro tenant) para um e-mail que já tem conta: o cadastro responde igual a token inventado (achado 7).
func (a *ambiente) testarCadastroSemEnumeracao(t *testing.T) {
	a.semear(t, "ten_a", "ja.tem.conta@exemplo.com", PapelAnalista)
	admin := a.entrar(t, a.semear(t, "ten_b", "admin.enum@exemplo.com", PapelAdmin))
	w := admin.fazer("POST", "/painel/equipe", `{"email":"ja.tem.conta@exemplo.com","papel":"analista"}`, true)
	quer(t, w, 201, "")
	var cv conviteJSON
	_ = json.Unmarshal(w.Body.Bytes(), &cv)
	link, _ := url.Parse(cv.Link)

	existente := a.novoCliente().fazer("POST", "/painel/auth/cadastro",
		`{"token":"`+link.Query().Get("token")+`","nome":"X","senha":"`+senhaTeste+`"}`, false)
	inventado := a.novoCliente().fazer("POST", "/painel/auth/cadastro",
		`{"token":"inventado","nome":"X","senha":"`+senhaTeste+`"}`, false)
	quer(t, existente, 404, "nao_encontrado")
	if existente.Code != inventado.Code || existente.Body.String() != inventado.Body.String() {
		t.Fatalf("respostas diferentes: %d %s / %d %s", existente.Code, existente.Body, inventado.Code, inventado.Body)
	}
}

// 30 min sem uso derrubam a sessão; usar antes disso empurra o prazo (achado 9).
func (a *ambiente) testarInatividade(t *testing.T) {
	u := a.semear(t, "ten_a", "inativo@exemplo.com", PapelAnalista)
	c := a.entrar(t, u)
	a.exec(`UPDATE sessoes SET ultimo_uso_em = now() - interval '29 minutes' WHERE usuario_id = $1`, u.id)
	quer(t, c.fazer("GET", "/painel/auth/eu", "", false), 200, "")
	var parado bool
	if err := a.b.Dono.QueryRow(context.Background(), `SELECT bool_or(ultimo_uso_em < now() - interval '1 minute')
		FROM sessoes WHERE usuario_id = $1 AND encerrada_em IS NULL`, u.id).Scan(&parado); err != nil || parado {
		t.Fatalf("uso não empurrou ultimo_uso_em (parado=%v, err=%v)", parado, err)
	}
	a.exec(`UPDATE sessoes SET ultimo_uso_em = now() - interval '31 minutes' WHERE usuario_id = $1`, u.id)
	quer(t, c.fazer("GET", "/painel/auth/eu", "", false), 401, "nao_autorizado")
}

// Achado 1: admin remove membro e troca papel, só no próprio tenant, sem nunca zerar os admins.
func (a *ambiente) testarRemoverETrocarPapel(t *testing.T) {
	a.exec(`INSERT INTO tenants (id, nome) VALUES ('ten_c', 'ten_c')`)
	admin := a.entrar(t, a.semear(t, "ten_c", "admin1.c@exemplo.com", PapelAdmin))
	admin2 := a.semear(t, "ten_c", "admin2.c@exemplo.com", PapelAdmin)
	analista := a.semear(t, "ten_c", "analista.c@exemplo.com", PapelAnalista)
	sessAnalista := a.entrar(t, analista)
	deB := a.semear(t, "ten_b", "membro.b@exemplo.com", PapelAnalista)
	euAdmin := strings.Split(strings.Split(admin.fazer("GET", "/painel/auth/eu", "", false).Body.String(), `"id":"`)[1], `"`)[0]

	// Não-admin não mexe na equipe.
	quer(t, sessAnalista.fazer("DELETE", "/painel/equipe/"+admin2.id, ``, true), 403, "proibido")
	quer(t, sessAnalista.fazer("PATCH", "/painel/equipe/"+admin2.id, `{"papel":"auditor"}`, true), 403, "proibido")
	// CSRF vale também aqui.
	quer(t, admin.fazer("DELETE", "/painel/equipe/"+analista.id, ``, false), 403, "csrf_invalido")

	// Membro de outro tenant: 404 e nada muda lá.
	quer(t, admin.fazer("DELETE", "/painel/equipe/"+deB.id, ``, true), 404, "nao_encontrado")
	quer(t, admin.fazer("PATCH", "/painel/equipe/"+deB.id, `{"papel":"admin"}`, true), 404, "nao_encontrado")
	quer(t, admin.fazer("DELETE", "/painel/equipe/usr_inexistente", ``, true), 404, "nao_encontrado")
	var papelB string
	if err := a.b.Dono.QueryRow(context.Background(), `SELECT papel FROM associacoes WHERE usuario_id = $1 AND tenant_id = 'ten_b'`,
		deB.id).Scan(&papelB); err != nil || papelB != "analista" {
		t.Fatalf("membro do B mudou: %q %v", papelB, err)
	}

	// Troca de papel vale na request seguinte da sessão do alvo.
	quer(t, admin.fazer("PATCH", "/painel/equipe/"+analista.id, `{"papel":"chefe"}`, true), 400, "corpo_invalido")
	w := admin.fazer("PATCH", "/painel/equipe/"+analista.id, `{"papel":"auditor"}`, true)
	quer(t, w, 200, "")
	if !strings.Contains(w.Body.String(), `"papel":"auditor"`) || !strings.Contains(w.Body.String(), `"usuario_id":"`+analista.id+`"`) {
		t.Fatalf("PATCH = %s", w.Body)
	}
	if eu := sessAnalista.fazer("GET", "/painel/auth/eu", "", false).Body.String(); !strings.Contains(eu, `"papel":"auditor"`) {
		t.Fatalf("papel novo não valeu na sessão: %s", eu)
	}

	// Remover derruba as sessões do removido na hora.
	quer(t, admin.fazer("DELETE", "/painel/equipe/"+analista.id, ``, true), 204, "")
	quer(t, sessAnalista.fazer("GET", "/painel/auth/eu", "", false), 401, "nao_autorizado")
	quer(t, admin.fazer("DELETE", "/painel/equipe/"+analista.id, ``, true), 404, "nao_encontrado")

	// Último admin: com dois, rebaixar um vale; depois o que sobrou não sai nem é rebaixado.
	quer(t, admin.fazer("PATCH", "/painel/equipe/"+admin2.id, `{"papel":"analista"}`, true), 200, "")
	quer(t, admin.fazer("PATCH", "/painel/equipe/"+euAdmin, `{"papel":"analista"}`, true), 409, "ultimo_admin")
	quer(t, admin.fazer("DELETE", "/painel/equipe/"+euAdmin, ``, true), 409, "ultimo_admin")
	quer(t, admin.fazer("PATCH", "/painel/equipe/"+euAdmin, `{"papel":"admin"}`, true), 200, "") // admin → admin não tira ninguém
	quer(t, admin.fazer("GET", "/painel/auth/eu", "", false), 200, "")
}

// Achado 1: chaves, IPs, equipe e auth deixam rastro na cadeia, sem e-mail em claro.
func (a *ambiente) testarAuditoriaDeAcesso(t *testing.T) {
	contar := func(tenantID, evento string) int {
		var n int
		if err := a.b.Dono.QueryRow(context.Background(), `SELECT count(*) FROM auditoria WHERE tenant_id = $1 AND evento = $2`,
			tenantID, evento).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	admin := a.entrar(t, a.semear(t, "ten_c", "audita.c@exemplo.com", PapelAdmin))
	w := admin.fazer("POST", "/painel/chaves", `{"nome":"para revogar"}`, true)
	quer(t, w, 201, "")
	var criada chaveCriadaJSON
	_ = json.Unmarshal(w.Body.Bytes(), &criada)
	quer(t, admin.fazer("DELETE", "/painel/chaves/"+criada.ID, ``, true), 204, "")
	quer(t, admin.fazer("DELETE", "/painel/chaves/"+criada.ID, ``, true), 204, "") // idempotente: audita uma vez só
	if n := contar("ten_c", auditoria.EventoChaveRevogada); n != 1 {
		t.Fatalf("revogações auditadas = %d; quer 1", n)
	}
	// Feitos pelos subtestes anteriores: chaves e IPs no ten_b; equipe no ten_a/ten_c; login, 2FA e sair no ten_a.
	for _, c := range []struct{ tenant, evento string }{
		{"ten_b", auditoria.EventoChaveCriada}, {"ten_b", auditoria.EventoChaveGirada},
		{"ten_c", auditoria.EventoChaveRevogada}, {"ten_b", auditoria.EventoIPsAlterados},
		{"ten_a", auditoria.EventoConviteCriado}, {"ten_a", auditoria.EventoConviteAceito},
		{"ten_c", auditoria.EventoPapelAlterado}, {"ten_c", auditoria.EventoMembroRemovido},
		{"ten_a", auditoria.EventoLoginOK}, {"ten_a", auditoria.EventoLoginFalhou},
		{"ten_a", auditoria.EventoDoisFatoresFalhou}, {"ten_a", auditoria.EventoSaiu},
	} {
		if contar(c.tenant, c.evento) == 0 {
			t.Errorf("nenhum %s auditado em %s", c.evento, c.tenant)
		}
	}
	var comEmail int
	if err := a.b.Dono.QueryRow(context.Background(), `SELECT count(*) FROM auditoria WHERE dados::text LIKE '%@%'`).Scan(&comEmail); err != nil {
		t.Fatal(err)
	}
	if comEmail != 0 {
		t.Fatalf("%d registros de auditoria com e-mail em claro", comEmail)
	}
	for _, ten := range []string{"ten_a", "ten_b", "ten_c"} {
		if err := tenant.EmTenant(context.Background(), a.b.App, ten, func(tx pgx.Tx) error {
			return auditoria.Verificar(context.Background(), tx, ten)
		}); err != nil {
			t.Fatalf("cadeia de %s: %v", ten, err)
		}
	}
}
