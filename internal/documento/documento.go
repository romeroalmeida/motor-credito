// Package documento normaliza, valida e mascara CPF e CNPJ (inclusive o CNPJ alfanumérico de 2026).
package documento

import (
	"errors"
	"strings"
)

// Tipo é o tipo do documento.
type Tipo string

const (
	CPF  Tipo = "cpf"
	CNPJ Tipo = "cnpj"
)

// ErrInvalido é devolvido para qualquer documento malformado ou com dígito verificador errado.
var ErrInvalido = errors.New("documento inválido")

// Normalizar remove máscara (. - / e espaços), põe letras em maiúscula e valida os dígitos verificadores.
// CNPJ alfanumérico: 12 primeiras posições [0-9A-Z], 2 últimas numéricas; valor do caractere = ASCII-48.
func Normalizar(entrada string) (string, Tipo, error) {
	var b strings.Builder
	for _, r := range strings.ToUpper(entrada) {
		switch {
		case r == '.' || r == '-' || r == '/' || r == ' ' || r == '\t':
		case (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z'):
			b.WriteRune(r)
		default:
			return "", "", ErrInvalido
		}
	}
	doc := b.String()
	if repetido(doc) {
		return "", "", ErrInvalido
	}
	switch len(doc) {
	case 11:
		if !numerico(doc) || !dvConfere(doc, 9) {
			return "", "", ErrInvalido
		}
		return doc, CPF, nil
	case 14:
		if !numerico(doc[12:]) || !dvConfere(doc, 12) {
			return "", "", ErrInvalido
		}
		return doc, CNPJ, nil
	}
	return "", "", ErrInvalido
}

// Mascarar esconde o documento para log: "***.456.789-**" (CPF) e "**.ABC.345/****-**" (CNPJ).
// Recebe o documento já normalizado; qualquer outra coisa vira "***".
func Mascarar(doc string) string {
	switch len(doc) {
	case 11:
		return "***." + doc[3:6] + "." + doc[6:9] + "-**"
	case 14:
		return "**." + doc[2:5] + "." + doc[5:8] + "/****-**"
	}
	return "***"
}

// dvConfere verifica os dois dígitos verificadores que seguem as n primeiras posições (módulo 11).
// CPF: pesos 10..2 e 11..2. CNPJ: pesos 5..2,9..2 e 6..2,9..2.
func dvConfere(doc string, n int) bool {
	for i := n; i < n+2; i++ {
		soma := 0
		for j := 0; j < i; j++ {
			soma += valor(doc[j]) * peso(n, i, j)
		}
		dv := 0
		if r := soma % 11; r >= 2 {
			dv = 11 - r
		}
		if valor(doc[i]) != dv {
			return false
		}
	}
	return true
}

func peso(n, i, j int) int {
	if n == 9 { // CPF
		return i + 1 - j
	}
	return (i-j-1)%8 + 2 // CNPJ: ciclo 2..9 da direita para a esquerda
}

// valor é ASCII-48 (Receita 2026): '0'..'9' → 0..9, 'A' → 17, 'Z' → 42. Não é base 36.
func valor(c byte) int { return int(c) - '0' }

func numerico(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func repetido(s string) bool {
	return s != "" && strings.Count(s, s[:1]) == len(s)
}
