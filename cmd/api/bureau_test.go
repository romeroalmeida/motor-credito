package main

import (
	"context"
	"testing"

	"github.com/romeroalmeida/motor-credito/internal/bureau"
)

func ambiente(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestMontarBureau(t *testing.T) {
	sobem := []map[string]string{
		{},
		{"BUREAU_CNPJ": "simulado", "BUREAU_CPF": "simulado"},
		{"BUREAU_CNPJ": " brasilapi "},
		{"BUREAU_CPF": "serpro-trial"},
		{"BUREAU_CPF": "serpro", "SERPRO_URL": "https://x", "SERPRO_CHAVE": "k"},
	}
	for _, m := range sobem {
		if b, err := montarBureau(ambiente(m)); err != nil || b == nil {
			t.Fatalf("%v: %v", m, err)
		}
	}
	recusam := []map[string]string{
		{"BUREAU_CNPJ": "brasilAPI"}, {"BUREAU_CNPJ": "serasa"}, {"BUREAU_CNPJ": "on"},
		{"BUREAU_CPF": "Serpro"}, {"BUREAU_CPF": "on"},
		{"BUREAU_CPF": "serpro"}, // produção sem credencial não cai no trial
		{"BUREAU_CPF": "serpro", "SERPRO_URL": "https://x"},
		{"BUREAU_CPF": "serpro", "SERPRO_CHAVE": "k"},
	}
	for _, m := range recusam {
		if _, err := montarBureau(ambiente(m)); err == nil {
			t.Fatalf("%v subiu; quer recusa", m)
		}
	}
	// CPF nunca sai para a rede sem BUREAU_CPF: com brasilapi, o CPF de fixture continua respondido pelo simulado.
	b, _ := montarBureau(ambiente(map[string]string{"BUREAU_CNPJ": "brasilapi"}))
	r, err := b.Consultar(context.Background(), bureau.CPFScoreAlto)
	if err != nil || r.Score != 920 || r.Fontes.Cadastro != bureau.FonteSimulado {
		t.Fatalf("CPF pelo composto: %+v %v", r, err)
	}
	// Serpro ligado e sem nascimento: não chama a rede, cadastro não verificado.
	b, _ = montarBureau(ambiente(map[string]string{"BUREAU_CPF": "serpro-trial"}))
	r, err = b.Consultar(context.Background(), bureau.CPFScoreAlto)
	if err != nil || r.Fontes.Cadastro != bureau.FonteNaoVerificado || r.Situacao != "" {
		t.Fatalf("CPF sem nascimento: %+v %v", r, err)
	}
}
