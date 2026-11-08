package tenant

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestLimitadorJanela(t *testing.T) {
	l := NovoLimitador()
	t0 := time.Unix(1_000_000, 0)
	for i, c := range []struct {
		em   time.Time
		quer bool
	}{
		{t0, true}, {t0.Add(10 * time.Second), true}, {t0.Add(59 * time.Second), false}, // limite 2
		{t0.Add(time.Minute), true}, // janela nova exatamente no fim
	} {
		ok, espera := l.Permitir("ten", 2, time.Minute, c.em)
		if ok != c.quer {
			t.Fatalf("uso %d: ok = %v; quer %v", i+1, ok, c.quer)
		}
		if !ok && espera != time.Second {
			t.Fatalf("espera = %v; quer 1s", espera)
		}
	}
	if ok, _ := l.Permitir("outro", 2, time.Minute, t0.Add(59*time.Second)); !ok {
		t.Fatal("chaves diferentes não podem dividir a janela")
	}
}

func TestSepararChave(t *testing.T) {
	id, seg := strings.Repeat("A", 26), strings.Repeat("f", 64)
	casos := map[string]bool{
		"ck_live_" + id + "_" + seg:       true,
		"ck_test_" + id + "_" + seg:       false,
		"ck_live_" + id + seg:             false,
		"ck_live_" + id + "_" + seg + "0": false,
		"ck_live_" + id[1:] + "_" + seg:   false,
		"":                                false,
	}
	for token, quer := range casos {
		if _, _, ok := separarChave(token); ok != quer {
			t.Errorf("separarChave(%q) = %v; quer %v", token, ok, quer)
		}
	}
}

// Comparação de segredo em tempo constante não é observável por um teste funcional; este guarda impede
// que alguém troque hmac.Equal/subtle.ConstantTimeCompare por bytes.Equal ou == nos arquivos do pacote.
func TestComparacaoDeSegredoEmTempoConstante(t *testing.T) {
	proibido := regexp.MustCompile(`bytes\.Equal\(|string\([a-zA-Z]*(hash|Hash|assinatura|Assinatura|segredo|Segredo)[a-zA-Z]*\)\s*[!=]=`)
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
		usosSeguros += strings.Count(string(fonte), "hmac.Equal(") + strings.Count(string(fonte), "subtle.ConstantTimeCompare(")
	}
	// assinatura, hash do segredo, hash da idempotência (ler e guardar)
	if usosSeguros < 4 {
		t.Errorf("esperava ≥ 4 comparações em tempo constante, achei %d", usosSeguros)
	}
}

func TestIPPermitido(t *testing.T) {
	lista, err := ParsearIPs([]string{"200.150.10.0/24", "2001:db8::/32"})
	if err != nil {
		t.Fatal(err)
	}
	casos := map[string]bool{
		"200.150.10.255:1": true, "200.150.11.0:1": false, "[2001:db8::5]:1": true,
		"[::ffff:200.150.10.1]:1": true, "sem-ip": false, "": false,
	}
	for remote, quer := range casos {
		if got := ipPermitido(remote, lista); got != quer {
			t.Errorf("ipPermitido(%q) = %v; quer %v", remote, got, quer)
		}
	}
	if !ipPermitido("qualquer", nil) {
		t.Error("lista vazia libera qualquer IP")
	}
}
