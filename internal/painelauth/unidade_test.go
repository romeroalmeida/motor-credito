package painelauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/romeroalmeida/motor-credito/internal/plataforma"
)

func TestCookieInseguroDuasTravas(t *testing.T) {
	casos := []struct {
		flag, ambiente string
		quer           bool
	}{
		{"on", "desenvolvimento", true},
		{"on", "teste", true},
		{"on", "producao", false},        // trava do ambiente
		{"on", "", false},                // ambiente ausente = produção
		{"on", "dev", false},             // desconhecido = produção
		{"", "desenvolvimento", false},   // trava da flag
		{"ON", "desenvolvimento", false}, // só "on" exato
		{"1", "desenvolvimento", false},
		{"true", "teste", false},
	}
	for _, c := range casos {
		if got := CookieInseguroPermitido(c.flag, c.ambiente); got != c.quer {
			t.Errorf("CookieInseguroPermitido(%q, %q) = %v; quer %v", c.flag, c.ambiente, got, c.quer)
		}
	}
}

func TestCookieSecurePorPadrao(t *testing.T) {
	for _, inseguro := range []bool{false, true} {
		h := Novo(nil, nil, Opcoes{CookieInseguro: inseguro})
		w := httptest.NewRecorder()
		h.definirCookies(w, tokensSessao{sessao: "s", csrf: "c"}, true)
		for _, ck := range w.Result().Cookies() {
			if ck.Secure == inseguro || ck.SameSite != http.SameSiteStrictMode {
				t.Errorf("inseguro=%v: cookie %s Secure=%v SameSite=%v", inseguro, ck.Name, ck.Secure, ck.SameSite)
			}
		}
	}
}

func TestSenhaArgon2id(t *testing.T) {
	h, err := hashSenha("correta-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("formato = %q", h)
	}
	h2, _ := hashSenha("correta-horse-battery")
	if h == h2 {
		t.Fatal("sal repetido")
	}
	casos := map[string]bool{"correta-horse-battery": true, "correta-horse-batterY": false, "": false}
	for senha, quer := range casos {
		if conferirSenha(senha, h) != quer {
			t.Errorf("conferirSenha(%q) != %v", senha, quer)
		}
	}
	for _, ruim := range []string{"", "$argon2i$v=19$m=1,t=1,p=1$AA$AA", strings.Replace(h, "argon2id", "bcrypt", 1), h[:len(h)-50]} {
		if conferirSenha("correta-horse-battery", ruim) {
			t.Errorf("hash malformado conferiu: %q", ruim)
		}
	}
}

func TestPassoDoCodigo(t *testing.T) {
	segredo, _, err := novoSegredoTOTP("x@exemplo.com")
	if err != nil {
		t.Fatal(err)
	}
	agora := time.Unix(1_800_000_000, 0)
	for desvio, quer := range map[time.Duration]bool{0: true, -30 * time.Second: true, 30 * time.Second: true, -61 * time.Second: false, 61 * time.Second: false} {
		codigo, _ := totp.GenerateCode(segredo, agora.Add(desvio))
		passo, ok := passoDoCodigo(segredo, codigo, agora)
		if ok != quer {
			t.Errorf("desvio %v: ok = %v; quer %v", desvio, ok, quer)
		}
		if ok && passo != agora.Add(desvio).Unix()/periodoTOTP {
			t.Errorf("desvio %v: passo = %d", desvio, passo)
		}
	}
	if _, ok := passoDoCodigo(segredo, "12345", agora); ok {
		t.Error("código de 5 dígitos aceito")
	}
}

func TestExigirPapel(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	casos := []struct {
		nome   string
		sessao *Sessao
		quer   int
	}{
		{"sem sessão", nil, 401},
		{"admin", &Sessao{Papel: PapelAdmin}, http.StatusTeapot},
		{"analista", &Sessao{Papel: PapelAnalista}, 403},
		{"auditor", &Sessao{Papel: PapelAuditor}, 403},
		{"papel vazio", &Sessao{}, 403},
	}
	for _, c := range casos {
		r := httptest.NewRequest("GET", "/", nil)
		if c.sessao != nil {
			r = r.WithContext(context.WithValue(r.Context(), chaveContexto{}, sessaoCarregada{Sessao: *c.sessao}))
		}
		w := httptest.NewRecorder()
		ExigirPapel(PapelAdmin)(ok).ServeHTTP(w, r)
		if w.Code != c.quer {
			t.Errorf("%s: status = %d; quer %d", c.nome, w.Code, c.quer)
		}
	}
}

func TestEscreveExigeCSRF(t *testing.T) {
	for metodo, quer := range map[string]bool{"GET": false, "HEAD": false, "OPTIONS": false, "POST": true, "PUT": true, "DELETE": true, "PATCH": true, "INVENTADO": true} {
		if escreve(metodo) != quer {
			t.Errorf("escreve(%s) != %v", metodo, quer)
		}
	}
}

// Comparação de segredo em tempo constante não é observável por teste funcional; este guarda impede que
// alguém troque subtle.ConstantTimeCompare por bytes.Equal ou == nos arquivos do pacote.
func TestComparacaoDeSegredoEmTempoConstante(t *testing.T) {
	proibido := regexp.MustCompile(`bytes\.Equal\(|(cabecalho|esperado|codigo|calculado|guardado|csrfHash|c\.Value)\s*[!=]=\s*[a-zA-Z]`)
	arquivos, _ := filepath.Glob("*.go")
	usosSeguros := 0
	for _, a := range arquivos {
		if strings.HasSuffix(a, "_test.go") {
			continue
		}
		fonte, err := os.ReadFile(a)
		if err != nil {
			t.Fatal(err)
		}
		if loc := proibido.FindIndex(fonte); loc != nil {
			t.Errorf("%s: comparação que vaza tempo: %q", a, fonte[loc[0]:loc[1]])
		}
		usosSeguros += strings.Count(string(fonte), "subtle.ConstantTimeCompare(")
	}
	// senha, código TOTP, CSRF cabeçalho×cookie, CSRF×sessão
	if usosSeguros < 4 {
		t.Errorf("esperava ≥ 4 comparações em tempo constante, achei %d", usosSeguros)
	}
}

// O segredo TOTP não pode abrir com a chave dos documentos nem ser copiado para outro usuário (achado 11).
func TestCifraTOTPSubchaveEAAD(t *testing.T) {
	mestra := make([]byte, 32)
	c, err := CifrarTOTP(mestra, "usr_A", "JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if s, err := DecifrarTOTP(mestra, "usr_A", c); err != nil || s != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("DecifrarTOTP = %q, %v", s, err)
	}
	if _, err := DecifrarTOTP(mestra, "usr_B", c); err == nil {
		t.Fatal("segredo de usr_A decifrou como usr_B")
	}
	if _, err := plataforma.Decifrar(mestra, c); err == nil {
		t.Fatal("segredo TOTP decifrou com a chave dos documentos")
	}
	if _, err := plataforma.DecifrarAAD(mestra, c, []byte("usr_A")); err == nil {
		t.Fatal("segredo TOTP decifrou com a chave mestra (sem subchave)")
	}
}

// No máximo cap(semArgon) argon2 ao mesmo tempo, por mais logins simultâneos que cheguem (achado 5).
func TestArgonComTetoDeConcorrencia(t *testing.T) {
	var mu sync.Mutex
	agora, pico := 0, 0
	original := argonIDKey
	t.Cleanup(func() { argonIDKey = original })
	argonIDKey = func(senha, sal []byte, it, mem uint32, p uint8, n uint32) []byte {
		mu.Lock()
		agora++
		pico = max(pico, agora)
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		agora--
		mu.Unlock()
		return make([]byte, n)
	}
	var wg sync.WaitGroup
	for range 4 * cap(semArgon) {
		wg.Go(func() {
			conferirSenha("senha", "$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHRzYWx0$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g")
		})
	}
	wg.Wait()
	if pico > cap(semArgon) || pico < 2 {
		t.Fatalf("pico de argon2 simultâneos = %d; teto %d", pico, cap(semArgon))
	}
}
