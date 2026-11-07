package plataforma

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

// ErrDecifrar cobre texto adulterado, curto demais ou chave errada (sem dizer qual).
var ErrDecifrar = errors.New("não foi possível decifrar")

// HashDoc é o HMAC-SHA256 do documento normalizado: serve para busca e velocidade sem guardar o documento.
func HashDoc(chave []byte, doc string) []byte {
	m := hmac.New(sha256.New, chave)
	m.Write([]byte(doc))
	return m.Sum(nil)
}

// Cifrar usa AES-256-GCM com nonce aleatório. Saída: nonce || texto cifrado || tag.
func Cifrar(chave, claro []byte) ([]byte, error) {
	aead, err := novoGCM(chave)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("gerar nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, claro, nil), nil
}

// Decifrar desfaz Cifrar e falha se o conteúdo foi alterado.
func Decifrar(chave, cifrado []byte) ([]byte, error) {
	aead, err := novoGCM(chave)
	if err != nil {
		return nil, err
	}
	n := aead.NonceSize()
	if len(cifrado) < n+aead.Overhead() {
		return nil, ErrDecifrar
	}
	claro, err := aead.Open(nil, cifrado[:n], cifrado[n:], nil)
	if err != nil {
		return nil, ErrDecifrar
	}
	return claro, nil
}

func novoGCM(chave []byte) (cipher.AEAD, error) {
	if len(chave) != 32 {
		return nil, fmt.Errorf("chave AES-256 precisa de 32 bytes, tem %d", len(chave))
	}
	bloco, err := aes.NewCipher(chave)
	if err != nil {
		return nil, fmt.Errorf("criar cifra: %w", err)
	}
	return cipher.NewGCM(bloco)
}
