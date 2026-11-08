package migrar_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/romeroalmeida/motor-credito/internal/plataforma/bancoteste"
	"github.com/romeroalmeida/motor-credito/internal/plataforma/migrar"
	"github.com/romeroalmeida/motor-credito/migrations"
)

func TestSchema(t *testing.T) {
	ctx := context.Background()
	b := bancoteste.Novo(t)

	// Dados de dois tenants, inseridos pelo dono (sem RLS).
	_, err := b.Dono.Exec(ctx, `
		INSERT INTO tenants (id, nome) VALUES ('ten_a', 'A'), ('ten_b', 'B');
		INSERT INTO usuarios (id, email, senha_hash) VALUES ('usr_a', 'a@x', 'h');
		INSERT INTO chaves_api (id, tenant_id, segredo_cifrado) VALUES ('k_b', 'ten_b', decode(repeat('ab', 32), 'hex'));
		INSERT INTO consultas (id, tenant_id, finalidade, base_legal, entrada, status, decisao) VALUES
			('cons_a', 'ten_a', 'concessao_credito', 'x', '{}', 'aprovado', '{}'),
			('cons_b', 'ten_b', 'concessao_credito', 'x', '{}', 'negado', '{}');
		INSERT INTO politicas (id, tenant_id, nome, rascunho) VALUES ('pol_a', 'ten_a', 'PF', '{}');
		INSERT INTO politica_versoes (politica_id, tenant_id, versao, conteudo, publicada_por) VALUES ('pol_a', 'ten_a', 1, '{}', 'usr_a');
		INSERT INTO auditoria (tenant_id, seq, evento, ator, dados, hash_anterior, hash) VALUES
			('ten_a', 1, 'teste', 'usuario:usr_a', '{}', decode(repeat('00', 32), 'hex'), decode(repeat('11', 32), 'hex'));`)
	if err != nil {
		t.Fatalf("preparar dados: %v", err)
	}

	// emTenant roda f numa transação do usuário da aplicação com app.tenant_id definido ("" = não define).
	emTenant := func(tenant string, f func(pgx.Tx) error) error {
		return pgx.BeginFunc(ctx, b.App, func(tx pgx.Tx) error {
			if tenant != "" {
				if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenant); err != nil {
					return err
				}
			}
			return f(tx)
		})
	}
	ids := func(tenant, sql string) []string {
		t.Helper()
		var out []string
		err := emTenant(tenant, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, sql)
			if err != nil {
				return err
			}
			out, err = pgx.CollectRows(rows, pgx.RowTo[string])
			return err
		})
		if err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return out
	}

	t.Run("migrations não reaplicam", func(t *testing.T) {
		aplicadas, err := migrar.Aplicar(ctx, b.Dono, migrations.FS)
		if err != nil || len(aplicadas) != 0 {
			t.Fatalf("segunda aplicação = %v, %v; quer nenhuma", aplicadas, err)
		}
	})

	t.Run("RLS nega leitura cruzada", func(t *testing.T) {
		if got := ids("ten_a", `SELECT id FROM consultas ORDER BY id`); strings.Join(got, ",") != "cons_a" {
			t.Fatalf("tenant A vê %v; quer só cons_a", got)
		}
		if got := ids("ten_a", `SELECT id FROM consultas WHERE id = 'cons_b'`); len(got) != 0 {
			t.Fatalf("tenant A leu consulta de B: %v", got)
		}
		if got := ids("ten_a", `SELECT id FROM chaves_api`); len(got) != 0 {
			t.Fatalf("tenant A leu chave de B: %v", got)
		}
		if got := ids("ten_a", `SELECT id FROM tenants`); strings.Join(got, ",") != "ten_a" {
			t.Fatalf("tenant A vê tenants %v", got)
		}
	})

	t.Run("sem app.tenant_id não vê nada", func(t *testing.T) {
		for _, tabela := range []string{"consultas", "tenants", "chaves_api", "politicas", "auditoria"} {
			if got := ids("", `SELECT 'x' FROM `+tabela); len(got) != 0 {
				t.Errorf("%s sem tenant devolveu %d linhas", tabela, len(got))
			}
		}
	})

	t.Run("RLS nega escrita cruzada", func(t *testing.T) {
		err := emTenant("ten_a", func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO consultas (id, tenant_id, finalidade, base_legal, entrada, status, decisao)
				VALUES ('cons_x', 'ten_b', 'f', 'b', '{}', 'aprovado', '{}')`)
			return err
		})
		if !violouRLS(err) {
			t.Fatalf("insert em outro tenant: erro = %v; quer violação de RLS", err)
		}
		var afetadas int64
		err = emTenant("ten_a", func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, `UPDATE consultas SET status = 'aprovado' WHERE id = 'cons_b'`)
			afetadas = tag.RowsAffected()
			return err
		})
		if err != nil || afetadas != 0 {
			t.Fatalf("update em consulta de B: %d linhas, %v", afetadas, err)
		}
	})

	t.Run("motor_app não altera auditoria nem versão de política", func(t *testing.T) {
		for _, sql := range []string{
			`UPDATE auditoria SET evento = 'adulterado'`,
			`DELETE FROM auditoria`,
			`UPDATE politica_versoes SET conteudo = '{"x":1}'`,
			`DELETE FROM politica_versoes`,
			`TRUNCATE auditoria`,
		} {
			err := emTenant("ten_a", func(tx pgx.Tx) error { _, err := tx.Exec(ctx, sql); return err })
			if codigo(err) != "42501" { // insufficient_privilege
				t.Errorf("%s: erro = %v; quer permissão negada (42501)", sql, err)
			}
		}
		err := emTenant("ten_a", func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO auditoria (tenant_id, seq, evento, ator, dados, hash_anterior, hash)
				VALUES ('ten_a', 2, 'teste', 'api:k', '{}', decode(repeat('11', 32), 'hex'), decode(repeat('22', 32), 'hex'))`)
			return err
		})
		if err != nil {
			t.Fatalf("insert na auditoria deveria passar: %v", err)
		}
	})

	t.Run("auditoria não bifurca", func(t *testing.T) {
		_, err := b.Dono.Exec(ctx, `INSERT INTO auditoria (tenant_id, seq, evento, ator, dados, hash_anterior, hash)
			VALUES ('ten_a', 9, 'teste', 'x', '{}', decode(repeat('00', 32), 'hex'), decode(repeat('33', 32), 'hex'))`)
		if codigo(err) != "23505" {
			t.Fatalf("segundo registro com o mesmo hash_anterior: erro = %v; quer unique_violation", err)
		}
	})

	t.Run("chave de API achada antes de saber o tenant", func(t *testing.T) {
		var tenant string
		err := emTenant("", func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT tenant_id FROM auth_chave_api('k_b')`).Scan(&tenant)
		})
		if err != nil || tenant != "ten_b" {
			t.Fatalf("auth_chave_api = %q, %v", tenant, err)
		}
	})
}

func codigo(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code
	}
	return ""
}

func violouRLS(err error) bool {
	return codigo(err) == "42501" && strings.Contains(err.Error(), "row-level security")
}
