package painelauth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// Parâmetros argon2id (mínimo recomendado pela OWASP: 19 MiB, 2 iterações, 1 via).
const (
	argonMemoriaKiB = 19 * 1024
	argonIteracoes  = 2
	argonVias       = 1
	argonTamanho    = 32
	argonSal        = 16
)

var b64 = base64.RawStdEncoding

// semArgon limita quantos argon2 rodam ao mesmo tempo (19 MiB cada): rajada de login espera na fila em vez de
// esgotar memória e CPU. argonIDKey é trocado nos testes.
var (
	semArgon   = make(chan struct{}, max(2, runtime.NumCPU()))
	argonIDKey = argon2.IDKey
)

func idKey(senha, sal []byte, t, m uint32, p uint8, tamanho uint32) []byte {
	semArgon <- struct{}{}
	defer func() { <-semArgon }()
	return argonIDKey(senha, sal, t, m, p, tamanho)
}

// hashSenha devolve o formato PHC: $argon2id$v=19$m=...,t=...,p=...$<sal>$<hash>.
func hashSenha(senha string) (string, error) {
	sal := make([]byte, argonSal)
	if _, err := rand.Read(sal); err != nil {
		return "", fmt.Errorf("gerar sal: %w", err)
	}
	h := idKey([]byte(senha), sal, argonIteracoes, argonMemoriaKiB, argonVias, argonTamanho)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemoriaKiB, argonIteracoes, argonVias,
		b64.EncodeToString(sal), b64.EncodeToString(h)), nil
}

// conferirSenha lê os parâmetros do próprio hash; hash malformado nunca confere.
func conferirSenha(senha, phc string) bool {
	partes := strings.Split(phc, "$")
	if len(partes) != 6 || partes[1] != "argon2id" || partes[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return false
	}
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || m == 0 || t == 0 || p == 0 {
		return false
	}
	sal, err1 := b64.DecodeString(partes[4])
	guardado, err2 := b64.DecodeString(partes[5])
	if err1 != nil || err2 != nil || len(guardado) == 0 {
		return false
	}
	calculado := idKey([]byte(senha), sal, t, m, p, uint32(len(guardado)))
	return subtle.ConstantTimeCompare(calculado, guardado) == 1
}

// hashFicticio é conferido quando o e-mail não existe, para o tempo de resposta não revelar quem tem conta.
var hashFicticio = sync.OnceValue(func() string {
	h, err := hashSenha("senha-ficticia-para-igualar-tempo")
	if err != nil {
		panic(err)
	}
	return h
})
