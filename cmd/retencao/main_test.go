package main

import "testing"

func TestDiasRetencao(t *testing.T) {
	casos := []struct {
		v    string
		quer int
		erro bool
	}{
		{"", 1825, false}, {" 30 ", 30, false}, {"1", 1, false},
		{"0", 0, true}, {"-5", 0, true}, {"5 anos", 0, true}, {"1e3", 0, true},
	}
	for _, c := range casos {
		d, err := diasRetencao(c.v)
		if d != c.quer || (err != nil) != c.erro {
			t.Errorf("diasRetencao(%q) = %d, %v; quer %d, erro=%v", c.v, d, err, c.quer, c.erro)
		}
	}
}
