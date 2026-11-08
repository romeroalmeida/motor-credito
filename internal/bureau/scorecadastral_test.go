package bureau

import (
	"reflect"
	"testing"
	"time"
)

var hojeScore = time.Date(2026, 11, 8, 15, 0, 0, 0, time.UTC)

func TestFatoresNasBordas(t *testing.T) {
	casos := []struct {
		nome   string
		fator  Fator
		codigo string
		pontos int
	}{
		{"ativa", fatorSituacao(Regular), "SITUACAO_ATIVA", 250},
		{"inativa", fatorSituacao(Inativo), "SITUACAO_INATIVA", 0},
		{"situação vazia", fatorSituacao(""), "SITUACAO_INATIVA", 0},

		{"aberta hoje", fatorTempo("2026-11-08", hojeScore), "TEMPO_EMPRESA_MENOS_1A", 0},
		{"1 ano menos 1 dia", fatorTempo("2025-11-09", hojeScore), "TEMPO_EMPRESA_MENOS_1A", 0},
		{"1 ano exato", fatorTempo("2025-11-08", hojeScore), "TEMPO_EMPRESA_1A_3A", 80},
		{"3 anos menos 1 dia", fatorTempo("2023-11-09", hojeScore), "TEMPO_EMPRESA_1A_3A", 80},
		{"3 anos exatos", fatorTempo("2023-11-08", hojeScore), "TEMPO_EMPRESA_3A_5A", 140},
		{"5 anos menos 1 dia", fatorTempo("2021-11-09", hojeScore), "TEMPO_EMPRESA_3A_5A", 140},
		{"5 anos exatos", fatorTempo("2021-11-08", hojeScore), "TEMPO_EMPRESA_5A_MAIS", 200},
		{"abertura futura", fatorTempo("2026-11-09", hojeScore), "TEMPO_EMPRESA_DESCONHECIDO", 0},
		{"abertura vazia", fatorTempo("", hojeScore), "TEMPO_EMPRESA_DESCONHECIDO", 0},
		{"abertura malformada", fatorTempo("08/11/2020", hojeScore), "TEMPO_EMPRESA_DESCONHECIDO", 0},

		{"capital zero", fatorCapital(0), "CAPITAL_ATE_10MIL", 0},
		{"capital 10 mil menos 1 centavo", fatorCapital(CapitalFaixa1 - 1), "CAPITAL_ATE_10MIL", 0},
		{"capital 10 mil", fatorCapital(CapitalFaixa1), "CAPITAL_10MIL_100MIL", 50},
		{"capital 100 mil menos 1 centavo", fatorCapital(CapitalFaixa2 - 1), "CAPITAL_10MIL_100MIL", 50},
		{"capital 100 mil", fatorCapital(CapitalFaixa2), "CAPITAL_100MIL_1MI", 100},
		{"capital 1 mi menos 1 centavo", fatorCapital(CapitalFaixa3 - 1), "CAPITAL_100MIL_1MI", 100},
		{"capital 1 mi", fatorCapital(CapitalFaixa3), "CAPITAL_1MI_MAIS", 150},
		{"capital negativo", fatorCapital(-1), "CAPITAL_ATE_10MIL", 0},

		{"porte micro", fatorPorte(1), "PORTE_MICRO", 40},
		{"porte pequeno", fatorPorte(3), "PORTE_PEQUENO", 70},
		{"porte demais", fatorPorte(5), "PORTE_DEMAIS", 100},
		{"porte 0", fatorPorte(0), "PORTE_NAO_INFORMADO", 0},
		{"porte desconhecido", fatorPorte(2), "PORTE_NAO_INFORMADO", 0},

		{"MEI", fatorRegime(true, true), "REGIME_MEI", 0},
		{"MEI sem Simples marcado", fatorRegime(false, true), "REGIME_MEI", 0},
		{"Simples", fatorRegime(true, false), "REGIME_SIMPLES", 50},
		{"regime normal", fatorRegime(false, false), "REGIME_NORMAL", 100},

		{"S.A. fechada", fatorNatureza(2054), "NATUREZA_SA", 150},
		{"economia mista", fatorNatureza(2038), "NATUREZA_SA", 150},
		{"limitada", fatorNatureza(2062), "NATUREZA_LIMITADA", 100},
		{"empresário individual", fatorNatureza(2135), "NATUREZA_INDIVIDUAL", 30},
		{"associação", fatorNatureza(3999), "NATUREZA_OUTRA", 0},
		{"natureza 0", fatorNatureza(0), "NATUREZA_OUTRA", 0},

		{"sem sócios", fatorSocios(0), "SOCIOS_NAO_INFORMADO", 0},
		{"1 sócio", fatorSocios(1), "SOCIO_1", 20},
		{"2 sócios", fatorSocios(2), "SOCIOS_2_MAIS", 50},
	}
	for _, c := range casos {
		if c.fator != (Fator{c.codigo, c.pontos}) {
			t.Errorf("%s: %+v, quer %s %+d", c.nome, c.fator, c.codigo, c.pontos)
		}
	}
}

func TestScoreCadastral(t *testing.T) {
	topo := Empresa{CapitalSocial: CapitalFaixa3, CodigoPorte: PorteDemais, Natureza: 2046, Socios: 3}
	score, fatores := ScoreCadastral(Regular, "2000-01-01", topo, hojeScore)
	if score != 1000 {
		t.Fatalf("empresa no topo de todos os fatores: %d, quer 1000 (%+v)", score, fatores)
	}
	quer := []Fator{
		{"SITUACAO_ATIVA", 250}, {"TEMPO_EMPRESA_5A_MAIS", 200}, {"CAPITAL_1MI_MAIS", 150}, {"PORTE_DEMAIS", 100},
		{"REGIME_NORMAL", 100}, {"NATUREZA_SA", 150}, {"SOCIOS_2_MAIS", 50},
	}
	if !reflect.DeepEqual(fatores, quer) {
		t.Fatalf("fatores %+v, quer %+v", fatores, quer)
	}

	// MEI recém-aberto: 250 (ativa) + 40 (micro) + 30 (individual) + 20 (1 sócio).
	mei := Empresa{CapitalSocial: 1_000_00, CodigoPorte: PorteMicro, Simples: true, MEI: true, Natureza: 2135, Socios: 1}
	if s, f := ScoreCadastral(Regular, "2026-06-01", mei, hojeScore); s != 340 {
		t.Fatalf("MEI novo: %d, quer 340 (%+v)", s, f)
	}
	if s, _ := ScoreCadastral(Inativo, "", Empresa{}, hojeScore); s != 100 { // só REGIME_NORMAL
		t.Fatalf("empresa vazia e inativa: %d, quer 100", s)
	}
	s1, f1 := ScoreCadastral(Regular, "2010-05-05", mei, hojeScore)
	s2, f2 := ScoreCadastral(Regular, "2010-05-05", mei, hojeScore)
	if s1 != s2 || !reflect.DeepEqual(f1, f2) {
		t.Fatal("não determinístico")
	}
}
