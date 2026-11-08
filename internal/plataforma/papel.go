package plataforma

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ConferirPapelDaApp recusa rodar (API e job de retenção) quando o usuário de DATABASE_URL passaria por cima da
// RLS: superusuário, BYPASSRLS ou dono (direto ou por herança de papel) de alguma tabela do schema public. A RLS não é FORCE,
// então o dono não sofre RLS; o isolamento entre tenants depende de a aplicação nunca conectar como ele.
func ConferirPapelDaApp(ctx context.Context, pool *pgxpool.Pool) error {
	var usuario string
	var super, bypass bool
	var donas int
	err := pool.QueryRow(ctx, `SELECT r.rolname, r.rolsuper, r.rolbypassrls,
			(SELECT count(*) FROM pg_tables t WHERE t.schemaname = 'public' AND pg_has_role(r.oid, t.tableowner, 'MEMBER'))
		FROM pg_roles r WHERE r.rolname = current_user`).Scan(&usuario, &super, &bypass, &donas)
	if err != nil {
		return fmt.Errorf("conferir papel do usuário da aplicação: %w", err)
	}
	var problemas []error
	if super {
		problemas = append(problemas, errors.New("é superusuário"))
	}
	if bypass {
		problemas = append(problemas, errors.New("tem BYPASSRLS"))
	}
	if donas > 0 {
		problemas = append(problemas, fmt.Errorf("é dono (ou herda o dono) de %d tabela(s)", donas))
	}
	if len(problemas) > 0 {
		return fmt.Errorf("DATABASE_URL conecta como %q, que escapa da RLS: %w; use um usuário membro de motor_app",
			usuario, errors.Join(problemas...))
	}
	return nil
}
