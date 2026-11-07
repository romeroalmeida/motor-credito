// Package migrar aplica os arquivos SQL de migrations/ em ordem de nome, cada um numa transação, uma vez só.
package migrar

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// chaveTrava é a advisory lock que impede duas instâncias migrando ao mesmo tempo.
const chaveTrava = 7_420_001

// Aplicar roda as migrations de arquivos (*.sql) ainda não registradas em schema_migracoes.
// Deve ser chamado com a conexão do DONO das tabelas, não com o usuário da aplicação.
func Aplicar(ctx context.Context, pool *pgxpool.Pool, arquivos fs.FS) ([]string, error) {
	nomes, err := fs.Glob(arquivos, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("listar migrations: %w", err)
	}
	sort.Strings(nomes)

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("obter conexão: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, chaveTrava); err != nil {
		return nil, fmt.Errorf("travar migrations: %w", err)
	}
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, chaveTrava) //nolint:errcheck // a trava cai com a conexão de qualquer jeito

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migracoes (
		versao text PRIMARY KEY, aplicada_em timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, fmt.Errorf("criar schema_migracoes: %w", err)
	}

	var aplicadas []string
	for _, nome := range nomes {
		versao := strings.TrimSuffix(nome, ".sql")
		var existe bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migracoes WHERE versao = $1)`, versao).Scan(&existe); err != nil {
			return aplicadas, fmt.Errorf("consultar %s: %w", versao, err)
		}
		if existe {
			continue
		}
		sql, err := fs.ReadFile(arquivos, nome)
		if err != nil {
			return aplicadas, fmt.Errorf("ler %s: %w", nome, err)
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migracoes (versao) VALUES ($1)`, versao)
			return err
		})
		if err != nil {
			return aplicadas, fmt.Errorf("aplicar %s: %w", versao, err)
		}
		aplicadas = append(aplicadas, versao)
	}
	return aplicadas, nil
}
