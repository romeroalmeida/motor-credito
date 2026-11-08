package dinheiro

import (
	"errors"
	"math"
	"testing"
)

// Referências calculadas à parte em Python (decimal com 80 dígitos, ROUND_HALF_UP).
func TestParcela(t *testing.T) {
	casos := []struct {
		nome        string
		valor       Centavos
		prazo, taxa int
		quer        Centavos
	}{
		{"6 mil 24x 2,90%", 600000, 24, 290, 35048},       // 35047,8977…
		{"10 mil 24x 5,90%", 1000000, 24, 590, 78944},     // 78944,2347…
		{"mil 12x 2,90%", 100000, 12, 290, 9986},          // 9986,3324…
		{"5 mil 36x 1,99%", 500000, 36, 199, 19585},       // 19584,9367…
		{"taxa mínima 0,01%", 12345, 7, 1, 1764},          // 1764,2769…
		{"500 mil 60x 1,50%", 50000000, 60, 150, 1269671}, // 1269671,3713…
		{"uma parcela", 100000, 1, 290, 102900},
		{"1 centavo arredonda para 0", 1, 12, 290, 0}, // 0,0998…
		{"taxa zero exata", 500, 4, 0, 125},
		{"taxa zero trunca abaixo de meio", 100, 3, 0, 33}, // 33,33…
		{"taxa zero meio sobe", 5, 2, 0, 3},                // 2,5 → 3 (não 2, que seria arredondamento bancário)
		{"taxa zero meio sobe 2", 25, 10, 0, 3},            // 2,5 → 3
		{"taxa zero acima de meio", 1000, 3, 0, 333},
		{"taxa zero 2/3", 2, 3, 0, 1}, // 0,666… → 1
		{"prazo zero", 100000, 0, 290, 0},
		{"prazo negativo", 100000, -1, 290, 0},
		{"valor zero", 0, 12, 290, 0},
		{"taxa negativa", 100000, 12, -1, 0},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got, err := Parcela(c.valor, c.prazo, c.taxa); err != nil || got != c.quer {
				t.Fatalf("Parcela(%d, %d, %d) = %d; quer %d", c.valor, c.prazo, c.taxa, got, c.quer)
			}
		})
	}
}

func TestParcelaPrazoLongoNaoEstoura(t *testing.T) {
	if got, err := Parcela(100000000, 420, 99); err != nil || got <= 0 {
		t.Fatalf("parcela de 420 meses = %d", got)
	}
}

// Achado 1 da conferência: MaxInt64 em 1x com taxa > 0 dava parcela negativa (q.Int64() sem checar).
func TestParcelaEstouraDevolveErro(t *testing.T) {
	casos := []struct {
		valor       Centavos
		prazo, taxa int
	}{{math.MaxInt64, 1, 290}, {9_000_000_000_000_000_000, 1, 290}, {math.MaxInt64, 1, 1}}
	for _, c := range casos {
		if got, err := Parcela(c.valor, c.prazo, c.taxa); !errors.Is(err, ErrEstouro) {
			t.Errorf("Parcela(%d, %d, %d) = %d, %v; quer ErrEstouro", c.valor, c.prazo, c.taxa, got, err)
		}
	}
	// Limite exato: MaxInt64 em 1x sem taxa cabe.
	if got, err := Parcela(math.MaxInt64, 1, 0); err != nil || got != math.MaxInt64 {
		t.Fatalf("Parcela(MaxInt64, 1, 0) = %d, %v", got, err)
	}
}
