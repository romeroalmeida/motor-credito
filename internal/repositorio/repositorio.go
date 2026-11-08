// Package repositorio é o acesso ao Postgres das tabelas de E (políticas, consultas, eventos de antifraude).
//
// Todo acesso por tenant passa por ParaTenant: o tenant entra no construtor, toda query filtra por ele e
// roda em tenant.EmTenant (SET LOCAL app.tenant_id), então a RLS é a segunda barreira.
package repositorio

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

// ErrNaoEncontrado cobre recurso inexistente e recurso de outro tenant (sempre 404).
var ErrNaoEncontrado = errors.New("não encontrado")

// Repo acessa os dados de um único tenant.
type Repo struct {
	pool     *pgxpool.Pool
	tenantID string
}

// ParaTenant amarra o repositório ao tenant (que vem da sessão ou da chave de API, nunca do cliente).
func ParaTenant(pool *pgxpool.Pool, tenantID string) *Repo {
	return &Repo{pool: pool, tenantID: tenantID}
}

func (r *Repo) emTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return tenant.EmTenant(ctx, r.pool, r.tenantID, fn)
}

func naoEncontrado(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNaoEncontrado
	}
	return err
}
