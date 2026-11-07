package plataforma

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NovoPool abre o pool pgx e confirma a conexão com um ping.
func NovoPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("abrir pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("conectar ao banco: %w", err)
	}
	return pool, nil
}
