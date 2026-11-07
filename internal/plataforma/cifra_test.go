package plataforma

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestHashDocRFC4231(t *testing.T) {
	// RFC 4231, caso 2: chave "Jefe", dado "what do ya want for nothing?".
	quer := "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843"
	if got := hex.EncodeToString(HashDoc([]byte("Jefe"), "what do ya want for nothing?")); got != quer {
		t.Fatalf("HashDoc = %s; quer %s", got, quer)
	}
}

func TestHashDocDependeDaChave(t *testing.T) {
	a := HashDoc([]byte("chave-a"), "52998224725")
	if !bytes.Equal(a, HashDoc([]byte("chave-a"), "52998224725")) {
		t.Fatal("mesma chave e doc devem dar o mesmo hash")
	}
	if bytes.Equal(a, HashDoc([]byte("chave-b"), "52998224725")) {
		t.Fatal("chave diferente deve dar hash diferente")
	}
}

func TestCifrarDecifrar(t *testing.T) {
	chave := bytes.Repeat([]byte{7}, 32)
	c1, err := Cifrar(chave, []byte("52998224725"))
	if err != nil {
		t.Fatal(err)
	}
	c2, _ := Cifrar(chave, []byte("52998224725"))
	if bytes.Equal(c1, c2) {
		t.Fatal("nonce repetido: duas cifras iguais")
	}
	if bytes.Contains(c1, []byte("52998224725")) {
		t.Fatal("texto claro aparece na cifra")
	}
	claro, err := Decifrar(chave, c1)
	if err != nil || string(claro) != "52998224725" {
		t.Fatalf("Decifrar = %q, %v", claro, err)
	}

	adulterado := bytes.Clone(c1)
	adulterado[len(adulterado)-1] ^= 1
	outra := bytes.Repeat([]byte{8}, 32)
	for nome, f := range map[string]func() ([]byte, error){
		"adulterado":   func() ([]byte, error) { return Decifrar(chave, adulterado) },
		"chave errada": func() ([]byte, error) { return Decifrar(outra, c1) },
		"curto":        func() ([]byte, error) { return Decifrar(chave, c1[:10]) },
	} {
		if _, err := f(); !errors.Is(err, ErrDecifrar) {
			t.Errorf("%s: erro = %v; quer ErrDecifrar", nome, err)
		}
	}
	if _, err := Cifrar(chave[:16], []byte("x")); err == nil {
		t.Error("chave de 16 bytes deveria ser recusada (só AES-256)")
	}
}

func TestNovoID(t *testing.T) {
	a, b := NovoID("cons"), NovoID("cons")
	if !strings.HasPrefix(a, "cons_") || len(a) != len("cons_")+26 {
		t.Fatalf("NovoID = %q", a)
	}
	if a == b || a > b {
		t.Fatalf("IDs devem ser únicos e crescentes: %q, %q", a, b)
	}
}
