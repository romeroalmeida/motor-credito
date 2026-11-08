package tenant

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

// Acima de limpezaAcima chaves vivas, a varredura roda no máximo uma vez por janela, não a cada chamada (achado 5).
func TestLimitadorLimpaNoMaximoUmaVezPorJanela(t *testing.T) {
	l := NovoLimitador()
	t0 := time.Unix(1_000_000, 0)
	for i := range limpezaAcima + 1 {
		l.Permitir(strconv.Itoa(i), 5, time.Minute, t0)
	}
	for i := range 1000 {
		l.Permitir("x"+strconv.Itoa(i), 5, time.Minute, t0.Add(time.Second))
	}
	if l.limpezas != 1 {
		t.Fatalf("limpezas = %d na mesma janela; quer 1", l.limpezas)
	}
	l.Permitir("depois", 5, time.Minute, t0.Add(2*time.Minute))
	if l.limpezas != 2 || len(l.janelas) != 1 {
		t.Fatalf("janela seguinte: limpezas = %d, janelas = %d; quer 2 e 1", l.limpezas, len(l.janelas))
	}
}

func TestIdDaChave(t *testing.T) {
	id, seg := strings.Repeat("A", 26), strings.Repeat("f", 64)
	casos := map[string]bool{
		"ck_live_" + id:             true,
		"ck_test_" + id:             false,
		"ck_live_" + id + "_" + seg: false, // formato antigo: o segredo não viaja mais no Bearer
		"ck_live_" + id[1:]:         false,
		"ck_live_" + id + "0":       false,
		"":                          false,
	}
	for token, quer := range casos {
		if _, ok := idDaChave(token); ok != quer {
			t.Errorf("idDaChave(%q) = %v; quer %v", token, ok, quer)
		}
	}
}

// Vetor fixo da assinatura: o mesmo que openapi, scripts/consulta-exemplo.sh e o e2e documentam.
func TestAssinarCobreIdempotencyKey(t *testing.T) {
	// printf '%s' '1700000000.pedido-1.{"a":1}' | openssl dgst -sha256 -hmac segredo
	const quer = "978ceb5cda878cb0610bead92cb859f485f02688364eb6f30b182abdbe92906a"
	a := Assinar("segredo", "1700000000", "pedido-1", []byte(`{"a":1}`))
	if a != quer {
		t.Fatalf("Assinar = %s; quer %s", a, quer)
	}
	if a == Assinar("segredo", "1700000000", "pedido-2", []byte(`{"a":1}`)) {
		t.Fatal("trocar a Idempotency-Key precisa mudar a assinatura")
	}
	if a == Assinar("segredo", "1700000000", "", []byte(`{"a":1}`)) {
		t.Fatal("tirar a Idempotency-Key precisa mudar a assinatura")
	}
	if a != Assinar("segredo", "1700000000", "pedido-1", []byte(`{"a":1}`)) {
		t.Fatal("assinatura precisa ser determinística")
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
	// assinatura, hash da idempotência (ler e guardar)
	if usosSeguros < 3 {
		t.Errorf("esperava ≥ 3 comparações em tempo constante, achei %d", usosSeguros)
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
