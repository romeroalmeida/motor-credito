package plataforma

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
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

// DerivarChave tira da chave mestra uma subchave de 32 bytes por finalidade (HKDF-SHA256): o segredo TOTP e o
// segredo da chave de API não usam a mesma chave dos documentos.
func DerivarChave(mestra []byte, finalidade string) []byte {
	k, err := hkdf.Key(sha256.New, mestra, nil, "motor-credito/"+finalidade, 32)
	if err != nil {
		panic(err) // só falha com tamanho de saída absurdo, que aqui é constante
	}
	return k
}

// Cifrar usa AES-256-GCM com nonce aleatório e sem AAD. Saída: nonce || texto cifrado || tag.
func Cifrar(chave, claro []byte) ([]byte, error) { return CifrarAAD(chave, claro, nil) }

// Decifrar desfaz Cifrar e falha se o conteúdo foi alterado.
func Decifrar(chave, cifrado []byte) ([]byte, error) { return DecifrarAAD(chave, cifrado, nil) }

// CifrarAAD amarra o texto cifrado a aad (ex.: o id da linha): o valor copiado para outra linha não decifra.
func CifrarAAD(chave, claro, aad []byte) ([]byte, error) {
	aead, err := novoGCM(chave)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("gerar nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, claro, aad), nil
}

// DecifrarAAD desfaz CifrarAAD; aad diferente do usado na cifra falha como adulteração.
func DecifrarAAD(chave, cifrado, aad []byte) ([]byte, error) {
	aead, err := novoGCM(chave)
	if err != nil {
		return nil, err
	}
	n := aead.NonceSize()
	if len(cifrado) < n+aead.Overhead() {
		return nil, ErrDecifrar
	}
	claro, err := aead.Open(nil, cifrado[:n], cifrado[n:], aad)
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
