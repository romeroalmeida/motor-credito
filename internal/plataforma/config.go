// Package plataforma reúne o que todo módulo usa: configuração, banco, IDs e cifra de documento.
package plataforma

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Ambientes reconhecidos. Qualquer valor ausente ou desconhecido conta como produção (fail-closed).
const (
	Desenvolvimento = "desenvolvimento"
	Teste           = "teste"
	Producao        = "producao"
)

// Config é a configuração do binário, lida só de variáveis de ambiente.
type Config struct {
	Ambiente            string // AMBIENTE; ausente ou desconhecido → "producao"
	Porta               string // PORTA; padrão "8080"
	DatabaseURL         string // DATABASE_URL: usuário membro de motor_app (obrigatória)
	DatabaseURLMigracao string // DATABASE_URL_MIGRACAO: dono das tabelas; vazio = não migra ao subir
	ChaveHMACDoc        []byte // CHAVE_HMAC_DOC: hex, ≥ 32 bytes (obrigatória)
	ChaveCifraDoc       []byte // CHAVE_CIFRA_DOC: hex, exatamente 32 bytes, AES-256 (obrigatória)
}

// EhProducao é verdadeiro salvo quando o ambiente é explicitamente desenvolvimento ou teste.
func (c Config) EhProducao() bool { return c.Ambiente == Producao }

// Carregar lê a configuração do processo. Erro aqui significa que o binário não deve subir.
func Carregar() (Config, error) { return CarregarDe(os.Getenv) }

// CarregarDe lê a configuração a partir de getenv (permite testar sem mexer no ambiente do processo).
func CarregarDe(getenv func(string) string) (Config, error) {
	c := Config{
		Ambiente:            Producao,
		Porta:               strings.TrimSpace(getenv("PORTA")),
		DatabaseURL:         strings.TrimSpace(getenv("DATABASE_URL")),
		DatabaseURLMigracao: strings.TrimSpace(getenv("DATABASE_URL_MIGRACAO")),
	}
	if a := strings.TrimSpace(getenv("AMBIENTE")); a == Desenvolvimento || a == Teste {
		c.Ambiente = a
	}
	if c.Porta == "" {
		c.Porta = "8080"
	}
	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL ausente"))
	}
	var err error
	if c.ChaveHMACDoc, err = chaveHex(getenv, "CHAVE_HMAC_DOC", 32, false); err != nil {
		errs = append(errs, err)
	}
	if c.ChaveCifraDoc, err = chaveHex(getenv, "CHAVE_CIFRA_DOC", 32, true); err != nil {
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("configuração inválida: %w", err)
	}
	return c, nil
}

func chaveHex(getenv func(string) string, nome string, tamanho int, exato bool) ([]byte, error) {
	v := strings.TrimSpace(getenv(nome))
	if v == "" {
		return nil, fmt.Errorf("%s ausente", nome)
	}
	b, err := hex.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("%s não é hexadecimal", nome)
	}
	if len(b) < tamanho || (exato && len(b) != tamanho) {
		return nil, fmt.Errorf("%s precisa de %d bytes, tem %d (gere com: openssl rand -hex 32)", nome, tamanho, len(b))
	}
	return b, nil
}
