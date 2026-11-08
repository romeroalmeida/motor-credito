package repositorio

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/romeroalmeida/motor-credito/internal/auditoria"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

// NovaConsulta é a linha gravada a cada decisão. Nada de documento em claro: hash e cifrado.
type NovaConsulta struct {
	ID              string
	DocHash         []byte  // nil quando o documento é inválido
	DocCifrado      []byte  // nil quando o documento é inválido
	DocTipo         *string // "cpf" | "cnpj"
	DispositivoHash []byte  // nil sem device_id
	Finalidade      string
	BaseLegal       string
	Entrada         []byte // JSON sem PII em claro
	Status          string
	Score           *int
	FaixaRisco      *string
	Decisao         []byte // JSON: resposta completa + trilha
	AntifraudeRisco *int
	PoliticaID      *string
	PoliticaVersao  *int
	IdempotencyKey  *string // preenchidos por GravarConsultaIdempotente
	IdempotencyHash []byte
	CriadoEm        time.Time
}

// GravarConsulta grava a consulta sem Idempotency-Key, o evento de velocidade e a auditoria numa transação só.
func (r *Repo) GravarConsulta(ctx context.Context, c NovaConsulta, ator string) error {
	if err := r.emTx(ctx, func(tx pgx.Tx) error { return r.inserir(ctx, tx, c, ator) }); err != nil {
		return fmt.Errorf("gravar consulta: %w", err)
	}
	return nil
}

// Espera por uma Idempotency-Key que outra requisição está decidindo.
const (
	intervaloTrava = 20 * time.Millisecond
	esperaMaxTrava = 10 * time.Second // muito acima do tempo máximo das fontes (2s)
)

// GravarConsultaIdempotente reserva (tenant, chave) numa advisory lock e, dentro dela, antes de decidir:
// chave já gravada com o mesmo hash de corpo → devolve a decisão gravada sem chamar decidir;
// com outro hash (ou hash nulo, de antes da 0003) → tenant.ErrIdempotenciaConflito; nada gravado → decidir() e grava.
// Assim o bureau roda no máximo uma vez por chave e nenhum corpo recebe a decisão de outro.
//
// A trava é tentada sem bloquear (pg_try_advisory_xact_lock): quem espera devolve a conexão ao pool entre as
// tentativas. Bloquear segurando a conexão esgotaria o pool com reenvios da mesma chave, e quem tem a trava
// precisa de outras conexões (política, contador antifraude) para decidir.
func (r *Repo) GravarConsultaIdempotente(ctx context.Context, chave string, hashCorpo []byte, ator string,
	decidir func() (NovaConsulta, error)) (decisaoExistente []byte, err error) {
	limite := time.Now().Add(esperaMaxTrava)
	for {
		travou := false
		err := r.emTx(ctx, func(tx pgx.Tx) error {
			if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended('idem:' || $1 || ':' || $2, 0))`,
				r.tenantID, chave).Scan(&travou); err != nil || !travou {
				return err
			}
			var hashGuardado []byte
			err := tx.QueryRow(ctx, `SELECT decisao, idempotency_hash FROM consultas WHERE tenant_id = $1 AND idempotency_key = $2`,
				r.tenantID, chave).Scan(&decisaoExistente, &hashGuardado)
			if err == nil {
				if subtle.ConstantTimeCompare(hashGuardado, hashCorpo) != 1 {
					decisaoExistente = nil
					return tenant.ErrIdempotenciaConflito
				}
				return nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			c, err := decidir()
			if err != nil {
				return err
			}
			c.IdempotencyKey, c.IdempotencyHash = &chave, hashCorpo
			return r.inserir(ctx, tx, c, ator)
		})
		if err != nil {
			return nil, fmt.Errorf("gravar consulta idempotente: %w", err)
		}
		if travou {
			return decisaoExistente, nil
		}
		if time.Now().After(limite) {
			return nil, errors.New("gravar consulta idempotente: chave ocupada por tempo demais")
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("gravar consulta idempotente: %w", ctx.Err())
		case <-time.After(intervaloTrava):
		}
	}
}

func (r *Repo) inserir(ctx context.Context, tx pgx.Tx, c NovaConsulta, ator string) error {
	if _, err := tx.Exec(ctx, `INSERT INTO consultas (id, tenant_id, doc_hash, doc_cifrado, doc_tipo, finalidade, base_legal,
			entrada, status, score, faixa_risco, decisao, antifraude_risco, politica_id, politica_versao, idempotency_key,
			idempotency_hash, criado_em)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11, $12::jsonb, $13, $14, $15, $16, $17, $18)`,
		c.ID, r.tenantID, c.DocHash, c.DocCifrado, c.DocTipo, c.Finalidade, c.BaseLegal, string(c.Entrada), c.Status,
		c.Score, c.FaixaRisco, string(c.Decisao), c.AntifraudeRisco, c.PoliticaID, c.PoliticaVersao, c.IdempotencyKey,
		c.IdempotencyHash, c.CriadoEm); err != nil {
		return fmt.Errorf("inserir consulta: %w", err)
	}
	if c.DocHash != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO antifraude_eventos (doc_hash, dispositivo_hash) VALUES ($1, $2)`,
			c.DocHash, c.DispositivoHash); err != nil {
			return fmt.Errorf("registrar evento de velocidade: %w", err)
		}
	}
	return auditoria.Registrar(ctx, tx, r.tenantID, auditoria.EventoConsultaDecidida, ator, &c.ID, map[string]any{
		"status": c.Status, "score": c.Score, "faixa_risco": c.FaixaRisco,
		"politica_id": c.PoliticaID, "politica_versao": c.PoliticaVersao,
	})
}

// LinhaConsulta é a consulta como está no banco (o documento continua cifrado).
type LinhaConsulta struct {
	ID              string
	DocCifrado      []byte
	DocTipo         *string
	Finalidade      string
	BaseLegal       string
	Entrada         []byte
	Status          string
	Score           *int
	FaixaRisco      *string
	Decisao         []byte
	AntifraudeRisco *int
	PoliticaVersao  *int
	CriadoEm        time.Time
}

// Filtro da listagem. Datas são dias no fuso de São Paulo (AAAA-MM-DD, já validadas).
type Filtro struct {
	Status, Faixa string
	DocHash       []byte
	Desde, Ate    string
	Cursor        string // id da última consulta da página anterior
	Limite        int
}

const colunasConsulta = `id, doc_cifrado, doc_tipo, finalidade, base_legal, entrada, status, score, faixa_risco,
	decisao, antifraude_risco, politica_versao, criado_em`

func lerLinha(l pgx.Row) (LinhaConsulta, error) {
	var c LinhaConsulta
	err := l.Scan(&c.ID, &c.DocCifrado, &c.DocTipo, &c.Finalidade, &c.BaseLegal, &c.Entrada, &c.Status, &c.Score,
		&c.FaixaRisco, &c.Decisao, &c.AntifraudeRisco, &c.PoliticaVersao, &c.CriadoEm)
	c.CriadoEm = c.CriadoEm.UTC()
	return c, err
}

// ListarConsultas devolve as mais recentes primeiro (o id é ULID, então ordenar por id é ordenar por tempo).
func (r *Repo) ListarConsultas(ctx context.Context, f Filtro) ([]LinhaConsulta, error) {
	sql := `SELECT ` + colunasConsulta + ` FROM consultas WHERE tenant_id = $1`
	args := []any{r.tenantID}
	cond := func(trecho string, v any) {
		args = append(args, v)
		sql += " AND " + trecho + "$" + strconv.Itoa(len(args))
	}
	if f.Status != "" {
		cond("status = ", f.Status)
	}
	if f.Faixa != "" {
		cond("faixa_risco = ", f.Faixa)
	}
	if f.DocHash != nil {
		cond("doc_hash = ", f.DocHash)
	}
	if f.Desde != "" {
		cond("criado_em >= (", f.Desde)
		sql += "::date)::timestamp AT TIME ZONE 'America/Sao_Paulo'"
	}
	if f.Ate != "" {
		cond("criado_em < (", f.Ate)
		sql += "::date + 1)::timestamp AT TIME ZONE 'America/Sao_Paulo'"
	}
	if f.Cursor != "" {
		cond("id < ", f.Cursor)
	}
	args = append(args, f.Limite)
	sql += " ORDER BY id DESC LIMIT $" + strconv.Itoa(len(args))

	var itens []LinhaConsulta
	err := r.emTx(ctx, func(tx pgx.Tx) error {
		linhas, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		itens, err = pgx.CollectRows(linhas, func(l pgx.CollectableRow) (LinhaConsulta, error) { return lerLinha(l) })
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("listar consultas: %w", err)
	}
	return itens, nil
}

// Consulta devolve uma consulta do tenant; de outro tenant ou inexistente → ErrNaoEncontrado.
func (r *Repo) Consulta(ctx context.Context, id string) (LinhaConsulta, error) {
	var c LinhaConsulta
	err := r.emTx(ctx, func(tx pgx.Tx) error {
		var err error
		c, err = lerLinha(tx.QueryRow(ctx, `SELECT `+colunasConsulta+` FROM consultas WHERE tenant_id = $1 AND id = $2`, r.tenantID, id))
		return err
	})
	if err != nil {
		return LinhaConsulta{}, naoEncontrado(err)
	}
	return c, nil
}
