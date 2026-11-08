package repositorio

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romeroalmeida/motor-credito/internal/auditoria"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

// RetencaoPadraoDias é o prazo de guarda do desenho §4 (5 anos).
const RetencaoPadraoDias = 1825

// AtorRetencao é quem assina o evento de retenção na auditoria.
const AtorRetencao = "sistema:retencao"

// AplicarRetencao anonimiza as consultas criadas antes de agora − dias, em todos os tenants: apaga o documento
// (cifrado e HMAC, que permitiria reidentificar pela busca) e a Idempotency-Key (texto livre do cliente) e mantém o
// agregado (status, score, faixa, valores, política e versão, decisão). Também apaga os eventos de velocidade
// mais velhos que o corte (têm o HMAC do documento) e, na mesma transação por tenant, as linhas vencidas da tabela
// idempotencia (chave do cliente, hash do corpo e resposta guardada). Idempotente: linha já anonimizada não conta
// de novo. O total devolvido conta só as consultas. Cada tenant com linhas apagadas ganha um evento "lgpd.retencao_aplicada" na auditoria, na mesma transação.
func AplicarRetencao(ctx context.Context, pool *pgxpool.Pool, dias int, agora time.Time) (int64, error) {
	if dias < 1 {
		return 0, fmt.Errorf("retenção de %d dias: precisa ser ao menos 1", dias)
	}
	antes := agora.AddDate(0, 0, -dias)
	linhas, err := pool.Query(ctx, `SELECT tenant_id FROM retencao_tenants_pendentes($1)`, antes)
	if err != nil {
		return 0, fmt.Errorf("listar tenants da retenção: %w", err)
	}
	tenants, err := pgx.CollectRows(linhas, pgx.RowTo[string])
	if err != nil {
		return 0, fmt.Errorf("listar tenants da retenção: %w", err)
	}
	var total int64
	for _, t := range tenants {
		err := tenant.EmTenant(ctx, pool, t, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, `UPDATE consultas SET doc_hash = NULL, doc_cifrado = NULL, idempotency_key = NULL,
					idempotency_hash = NULL, dados_apagados_em = $3
				WHERE tenant_id = $1 AND criado_em < $2 AND dados_apagados_em IS NULL`, t, antes, agora)
			if err != nil {
				return err
			}
			n := tag.RowsAffected()
			tag, err = tx.Exec(ctx, `DELETE FROM idempotencia WHERE tenant_id = $1 AND criado_em < $2`, t, antes)
			if err != nil {
				return err
			}
			m := tag.RowsAffected()
			total += n
			if n == 0 && m == 0 {
				return nil
			}
			return auditoria.Registrar(ctx, tx, t, auditoria.EventoRetencaoAplicada, AtorRetencao, nil, map[string]any{
				"consultas": n, "idempotencia": m, "criadas_antes_de": antes.UTC().Format(time.RFC3339), "retencao_dias": dias,
			})
		})
		if err != nil {
			return total, fmt.Errorf("retenção no tenant %s: %w", t, err)
		}
	}
	if _, err := pool.Exec(ctx, `DELETE FROM antifraude_eventos WHERE criado_em < $1`, antes); err != nil {
		return total, fmt.Errorf("apagar eventos de velocidade antigos: %w", err)
	}
	return total, nil
}
