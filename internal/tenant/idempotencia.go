package tenant

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// MaxChaveIdempotencia é o tamanho máximo do cabeçalho Idempotency-Key.
const MaxChaveIdempotencia = 255

var (
	// ErrIdempotenciaConflito: mesma Idempotency-Key com corpo diferente (409 conflito_idempotencia).
	ErrIdempotenciaConflito = errors.New("idempotency-key reutilizada com corpo diferente")
	// ErrIdempotenciaChaveInvalida: chave vazia ou acima de MaxChaveIdempotencia (400).
	ErrIdempotenciaChaveInvalida = errors.New("idempotency-key inválida")
)

// Idempotencia procura uma resposta guardada para (tenant, chave). Mesmo corpo → (resposta, true);
// corpo diferente → ErrIdempotenciaConflito; nada guardado → (nil, false, nil).
func (s *Servico) Idempotencia(ctx context.Context, tenantID, chave string, hashCorpo []byte) ([]byte, bool, error) {
	if err := validarChaveIdempotencia(chave); err != nil {
		return nil, false, err
	}
	var hashGuardado, resposta []byte
	err := EmTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT hash_corpo, resposta FROM idempotencia WHERE tenant_id = $1 AND chave = $2`,
			tenantID, chave).Scan(&hashGuardado, &resposta)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("ler idempotência: %w", err)
	}
	if subtle.ConstantTimeCompare(hashGuardado, hashCorpo) != 1 {
		return nil, false, ErrIdempotenciaConflito
	}
	return resposta, true, nil
}

// GuardarIdempotencia grava a resposta. Se outra requisição já gravou a mesma chave, vale a primeira;
// se o corpo dela era diferente, devolve ErrIdempotenciaConflito.
func (s *Servico) GuardarIdempotencia(ctx context.Context, tenantID, chave string, hashCorpo, resposta []byte) error {
	if err := validarChaveIdempotencia(chave); err != nil {
		return err
	}
	var hashGuardado []byte
	err := EmTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO idempotencia (tenant_id, chave, hash_corpo, resposta) VALUES ($1, $2, $3, $4)
			ON CONFLICT (tenant_id, chave) DO NOTHING`, tenantID, chave, hashCorpo, resposta)
		if err != nil || tag.RowsAffected() == 1 {
			return err
		}
		return tx.QueryRow(ctx, `SELECT hash_corpo FROM idempotencia WHERE tenant_id = $1 AND chave = $2`,
			tenantID, chave).Scan(&hashGuardado)
	})
	if err != nil {
		return fmt.Errorf("guardar idempotência: %w", err)
	}
	if hashGuardado != nil && subtle.ConstantTimeCompare(hashGuardado, hashCorpo) != 1 {
		return ErrIdempotenciaConflito
	}
	return nil
}

func validarChaveIdempotencia(chave string) error {
	if chave == "" || len(chave) > MaxChaveIdempotencia {
		return ErrIdempotenciaChaveInvalida
	}
	return nil
}
