package documento

import (
	"errors"
	"testing"
)

func TestNormalizar(t *testing.T) {
	casos := []struct {
		nome, entrada, doc string
		tipo               Tipo
		ok                 bool
	}{
		{"cpf com máscara", "529.982.247-25", "52998224725", CPF, true},
		{"cpf sem máscara", "52998224725", "52998224725", CPF, true},
		{"cpf resto 2 vira dv 9", "123.456.789-09", "12345678909", CPF, true},
		{"cpf resto 2 no primeiro dv", "000.000.001-91", "00000000191", CPF, true},
		{"cpf restos 0 e 1 viram dv 0", "987.654.321-00", "98765432100", CPF, true},
		{"cpf com espaços", " 529 982 247 25 ", "52998224725", CPF, true},
		{"cpf dv1 errado", "529.982.247-35", "", "", false},
		{"cpf dv2 errado", "529.982.247-26", "", "", false},
		{"cpf repetido", "111.111.111-11", "", "", false},
		{"cpf com letra", "5299822472A", "", "", false},
		{"cpf curto", "5299822472", "", "", false},
		{"cnpj numérico", "11.222.333/0001-81", "11222333000181", CNPJ, true},
		{"cnpj numérico dv errado", "11.222.333/0001-82", "", "", false},
		{"cnpj alfanumérico oficial", "12.ABC.345/01DE-35", "12ABC34501DE35", CNPJ, true},
		{"cnpj alfanumérico minúsculo", "12abc34501de35", "12ABC34501DE35", CNPJ, true},
		{"cnpj alfanumérico dv errado", "12ABC34501DE36", "", "", false},
		{"cnpj com letra no dv", "12ABC34501DE3A", "", "", false},
		{"cnpj repetido", "00000000000000", "", "", false},
		{"cnpj repetido letra", "AAAAAAAAAAAAAA", "", "", false},
		{"caractere estranho", "529.982.247_25", "", "", false},
		{"acentuado", "Á2ABC34501DE35", "", "", false},
		{"vazio", "", "", "", false},
		{"só máscara", "..-/", "", "", false},
		{"tamanho 12", "123456789012", "", "", false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			doc, tipo, err := Normalizar(c.entrada)
			if c.ok {
				if err != nil || doc != c.doc || tipo != c.tipo {
					t.Fatalf("Normalizar(%q) = %q, %q, %v; quer %q, %q, nil", c.entrada, doc, tipo, err, c.doc, c.tipo)
				}
				return
			}
			if !errors.Is(err, ErrInvalido) || doc != "" {
				t.Fatalf("Normalizar(%q) = %q, %v; quer ErrInvalido", c.entrada, doc, err)
			}
		})
	}
}

// A letra vale ASCII-48 (A=17), não base 36 (A=10): este CNPJ só é válido pela regra da Receita.
func TestCNPJAlfanumericoUsaASCIIMenos48(t *testing.T) {
	if _, _, err := Normalizar("12ABC34501DE35"); err != nil {
		t.Fatalf("exemplo oficial recusado: %v", err)
	}
	if valor('A') != 17 || valor('Z') != 42 || valor('0') != 0 {
		t.Fatalf("valor: A=%d Z=%d 0=%d", valor('A'), valor('Z'), valor('0'))
	}
}

func TestMascarar(t *testing.T) {
	casos := map[string]string{
		"52998224725":    "***.982.247-**",
		"12345678909":    "***.456.789-**",
		"12ABC34501DE35": "**.ABC.345/****-**",
		"":               "***",
		"123":            "***",
	}
	for doc, quer := range casos {
		if got := Mascarar(doc); got != quer {
			t.Errorf("Mascarar(%q) = %q; quer %q", doc, got, quer)
		}
	}
}
