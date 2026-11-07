package plataforma

import (
	"strings"
	"testing"
)

const hex32 = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func ambiente(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func valido() map[string]string {
	return map[string]string{"DATABASE_URL": "postgres://x", "CHAVE_HMAC_DOC": hex32, "CHAVE_CIFRA_DOC": hex32}
}

func TestCarregarFailClosed(t *testing.T) {
	casos := []struct {
		nome, chave, valor, erro string
	}{
		{"sem DATABASE_URL", "DATABASE_URL", "", "DATABASE_URL ausente"},
		{"sem CHAVE_HMAC_DOC", "CHAVE_HMAC_DOC", "", "CHAVE_HMAC_DOC ausente"},
		{"sem CHAVE_CIFRA_DOC", "CHAVE_CIFRA_DOC", "", "CHAVE_CIFRA_DOC ausente"},
		{"hmac não hex", "CHAVE_HMAC_DOC", "zz", "não é hexadecimal"},
		{"hmac curta 31 bytes", "CHAVE_HMAC_DOC", hex32[:62], "precisa de 32 bytes"},
		{"cifra 31 bytes", "CHAVE_CIFRA_DOC", hex32[:62], "precisa de 32 bytes"},
		{"cifra 33 bytes", "CHAVE_CIFRA_DOC", hex32 + "20", "precisa de 32 bytes"},
		{"só espaços", "DATABASE_URL", "   ", "DATABASE_URL ausente"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			m := valido()
			m[c.chave] = c.valor
			_, err := CarregarDe(ambiente(m))
			if err == nil || !strings.Contains(err.Error(), c.erro) {
				t.Fatalf("erro = %v; quer conter %q", err, c.erro)
			}
		})
	}
}

func TestCarregarValido(t *testing.T) {
	m := valido()
	m["CHAVE_HMAC_DOC"] = hex32 + hex32 // HMAC aceita chave maior
	c, err := CarregarDe(ambiente(m))
	if err != nil {
		t.Fatal(err)
	}
	if c.Porta != "8080" || len(c.ChaveCifraDoc) != 32 || len(c.ChaveHMACDoc) != 64 {
		t.Fatalf("config = %+v", c)
	}
}

func TestAmbienteDesconhecidoEhProducao(t *testing.T) {
	casos := map[string]bool{"": true, "producao": true, "dev": true, "PRODUCAO": true, "desenvolvimento": false, "teste": false}
	for valor, producao := range casos {
		m := valido()
		m["AMBIENTE"] = valor
		c, err := CarregarDe(ambiente(m))
		if err != nil {
			t.Fatal(err)
		}
		if c.EhProducao() != producao {
			t.Errorf("AMBIENTE=%q: EhProducao = %v; quer %v", valor, c.EhProducao(), producao)
		}
	}
}
