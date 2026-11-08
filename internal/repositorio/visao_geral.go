package repositorio

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// Períodos aceitos pela visão geral.
var Periodos = map[string]bool{"hoje": true, "7d": true, "30d": true}

const (
	larguraFaixaScore = 100
	scoreMaximo       = 1000
	maxSinais         = 10
)

// FaixaScore é um balde da distribuição de score.
type FaixaScore struct {
	De         int `json:"de"`
	Ate        int `json:"ate"`
	Quantidade int `json:"quantidade"`
}

// SinalContado é um sinal de antifraude e quantas vezes apareceu.
type SinalContado struct {
	Sinal      string `json:"sinal"`
	Quantidade int    `json:"quantidade"`
}

// VisaoGeral são os números do período (contrato VisaoGeral do OpenAPI).
type VisaoGeral struct {
	Periodo           string         `json:"periodo"`
	Total             int            `json:"total"`
	PorStatus         map[string]int `json:"por_status"`
	TaxaAprovacaoBps  int            `json:"taxa_aprovacao_bps"`     // (aprovado + contraproposta) / consultas com pedido
	TaxaPreAprovacao  int            `json:"taxa_pre_aprovacao_bps"` // pre_aprovado / consultas sem pedido (de limite)
	DistribuicaoScore []FaixaScore   `json:"distribuicao_score"`
	SinaisFrequentes  []SinalContado `json:"sinais_frequentes"`
}

// VisaoGeral agrega as consultas do período ("hoje" é o dia de São Paulo).
func (r *Repo) VisaoGeral(ctx context.Context, periodo string) (VisaoGeral, error) {
	if !Periodos[periodo] {
		return VisaoGeral{}, fmt.Errorf("período desconhecido %q", periodo)
	}
	v := VisaoGeral{Periodo: periodo, PorStatus: map[string]int{}, SinaisFrequentes: []SinalContado{}}
	for de := 0; de < scoreMaximo; de += larguraFaixaScore {
		ate := de + larguraFaixaScore - 1
		if de+larguraFaixaScore == scoreMaximo {
			ate = scoreMaximo
		}
		v.DistribuicaoScore = append(v.DistribuicaoScore, FaixaScore{De: de, Ate: ate})
	}
	var comPedido int
	err := r.emTx(ctx, func(tx pgx.Tx) error {
		var desde time.Time
		if err := tx.QueryRow(ctx, `SELECT CASE $1
			WHEN 'hoje' THEN date_trunc('day', now() AT TIME ZONE 'America/Sao_Paulo') AT TIME ZONE 'America/Sao_Paulo'
			WHEN '7d' THEN now() - interval '7 days'
			ELSE now() - interval '30 days' END`, periodo).Scan(&desde); err != nil {
			return err
		}
		if err := contar(ctx, tx, `SELECT status, count(*) FROM consultas WHERE tenant_id = $1 AND criado_em >= $2 GROUP BY status`,
			r.tenantID, desde, func(status string, n int) { v.PorStatus[status] = n; v.Total += n }); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM consultas WHERE tenant_id = $1 AND criado_em >= $2
			AND jsonb_typeof(decisao -> 'pedido') = 'object'`, r.tenantID, desde).Scan(&comPedido); err != nil {
			return err
		}
		if err := contar(ctx, tx, `SELECT least(score / 100, 9)::text, count(*) FROM consultas
			WHERE tenant_id = $1 AND criado_em >= $2 AND score IS NOT NULL GROUP BY 1`,
			r.tenantID, desde, func(balde string, n int) {
				if i, err := strconv.Atoi(balde); err == nil && i >= 0 && i < len(v.DistribuicaoScore) {
					v.DistribuicaoScore[i].Quantidade = n
				}
			}); err != nil {
			return err
		}
		return contar(ctx, tx, `SELECT s, count(*) FROM consultas,
				jsonb_array_elements_text(coalesce(decisao -> 'antifraude' -> 'sinais', '[]'::jsonb)) s
			WHERE tenant_id = $1 AND criado_em >= $2 GROUP BY s ORDER BY 2 DESC, 1 LIMIT `+fmt.Sprint(maxSinais),
			r.tenantID, desde, func(s string, n int) { v.SinaisFrequentes = append(v.SinaisFrequentes, SinalContado{s, n}) })
	})
	if err != nil {
		return VisaoGeral{}, fmt.Errorf("visão geral: %w", err)
	}
	// Pré-aprovação não é aprovação: cada taxa tem o próprio denominador (com pedido × consulta de limite).
	if comPedido > 0 {
		v.TaxaAprovacaoBps = (v.PorStatus["aprovado"] + v.PorStatus["contraproposta"]) * 10_000 / comPedido
	}
	if semPedido := v.Total - comPedido; semPedido > 0 {
		v.TaxaPreAprovacao = v.PorStatus["pre_aprovado"] * 10_000 / semPedido
	}
	return v, nil
}

func contar(ctx context.Context, tx pgx.Tx, sql, tenantID string, desde time.Time, fn func(string, int)) error {
	linhas, err := tx.Query(ctx, sql, tenantID, desde)
	if err != nil {
		return err
	}
	var chave string
	var n int
	_, err = pgx.ForEachRow(linhas, []any{&chave, &n}, func() error { fn(chave, n); return nil })
	return err
}
