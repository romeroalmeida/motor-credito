// Package auditoria grava eventos com hash encadeado por tenant e verifica a cadeia.
//
// hash = SHA-256(hash_anterior || JSON canônico de {tenant, seq, evento, ator, consulta, dados}).
// O primeiro registro de cada tenant usa hash_anterior = 32 bytes zero. A tabela é só de inserção para o
// usuário da aplicação; qualquer alteração feita por fora (dono do banco) quebra a cadeia em Verificar.
package auditoria

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Eventos gravados pela aplicação.
const (
	EventoConsultaDecidida  = "consulta.decidida"
	EventoPoliticaPublicada = "politica.publicada"
)

var hashInicial = make([]byte, sha256.Size)

// ErrCadeiaQuebrada indica registro alterado, removido ou fora de ordem.
var ErrCadeiaQuebrada = errors.New("cadeia de auditoria quebrada")

// AtorAPI identifica uma chamada da API pública pela chave usada.
func AtorAPI(chaveID string) string { return "api:" + chaveID }

// AtorUsuario identifica uma ação feita no painel.
func AtorUsuario(usuarioID string) string { return "usuario:" + usuarioID }

// Registrar acrescenta um evento à cadeia do tenant. Rode dentro da mesma transação da ação auditada
// (tx já com app.tenant_id definido): se a ação desfizer, o registro some junto.
func Registrar(ctx context.Context, tx pgx.Tx, tenantID, evento, ator string, consultaID *string, dados any) error {
	// Serializa a cadeia do tenant. Não dá para usar FOR UPDATE: o usuário da aplicação não tem UPDATE na tabela.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('auditoria:' || $1, 0))`, tenantID); err != nil {
		return fmt.Errorf("travar auditoria: %w", err)
	}
	seq, anterior := int64(0), hashInicial
	err := tx.QueryRow(ctx, `SELECT seq, hash FROM auditoria WHERE tenant_id = $1 ORDER BY seq DESC LIMIT 1`, tenantID).
		Scan(&seq, &anterior)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("ler fim da cadeia: %w", err)
	}
	bruto, err := json.Marshal(dados)
	if err != nil {
		return fmt.Errorf("serializar dados da auditoria: %w", err)
	}
	canon, err := canonico(bruto)
	if err != nil {
		return err
	}
	seq++
	h, err := calcular(anterior, tenantID, seq, evento, ator, consultaID, canon)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO auditoria (tenant_id, seq, evento, ator, consulta_id, dados, hash_anterior, hash)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)`, tenantID, seq, evento, ator, consultaID, string(canon), anterior, h)
	if err != nil {
		return fmt.Errorf("gravar auditoria: %w", err)
	}
	return nil
}

// Consultor é o que Verificar precisa: serve tanto *pgxpool.Pool quanto pgx.Tx.
type Consultor interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Verificar percorre a cadeia do tenant e devolve ErrCadeiaQuebrada (com o seq) no primeiro registro que não confere.
func Verificar(ctx context.Context, q Consultor, tenantID string) error {
	linhas, err := q.Query(ctx, `SELECT seq, evento, ator, consulta_id, dados, hash_anterior, hash
		FROM auditoria WHERE tenant_id = $1 ORDER BY seq`, tenantID)
	if err != nil {
		return fmt.Errorf("ler auditoria: %w", err)
	}
	defer linhas.Close()
	esperadoSeq, anterior := int64(1), hashInicial
	for linhas.Next() {
		var seq int64
		var evento, ator string
		var consultaID *string
		var dados, hashAnterior, hash []byte
		if err := linhas.Scan(&seq, &evento, &ator, &consultaID, &dados, &hashAnterior, &hash); err != nil {
			return fmt.Errorf("ler registro de auditoria: %w", err)
		}
		if seq != esperadoSeq || !bytes.Equal(hashAnterior, anterior) {
			return fmt.Errorf("%w: seq %d fora de ordem ou desligado do anterior", ErrCadeiaQuebrada, seq)
		}
		canon, err := canonico(dados)
		if err != nil {
			return fmt.Errorf("%w: seq %d com dados ilegíveis", ErrCadeiaQuebrada, seq)
		}
		calculado, err := calcular(anterior, tenantID, seq, evento, ator, consultaID, canon)
		if err != nil {
			return err
		}
		if !bytes.Equal(calculado, hash) {
			return fmt.Errorf("%w: seq %d alterado", ErrCadeiaQuebrada, seq)
		}
		esperadoSeq, anterior = seq+1, hash
	}
	return linhas.Err()
}

// Registro é uma linha da cadeia como o painel mostra. HashOK confere só a própria linha (conteúdo × hash);
// a ligação entre linhas é conferida por Verificar.
type Registro struct {
	Seq        int64     `json:"seq"`
	Evento     string    `json:"evento"`
	Ator       string    `json:"ator"`
	ConsultaID *string   `json:"consulta_id"`
	CriadoEm   time.Time `json:"criado_em"`
	HashOK     bool      `json:"hash_ok"`
}

// Listar devolve até limite registros do tenant, do mais novo para o mais velho, com seq < antesDe (0 = do fim).
func Listar(ctx context.Context, q Consultor, tenantID string, antesDe int64, limite int) ([]Registro, error) {
	linhas, err := q.Query(ctx, `SELECT seq, evento, ator, consulta_id, dados, hash_anterior, hash, criado_em
		FROM auditoria WHERE tenant_id = $1 AND ($2 = 0 OR seq < $2) ORDER BY seq DESC LIMIT $3`, tenantID, antesDe, limite)
	if err != nil {
		return nil, fmt.Errorf("listar auditoria: %w", err)
	}
	defer linhas.Close()
	itens := []Registro{}
	for linhas.Next() {
		var r Registro
		var dados, anterior, hash []byte
		if err := linhas.Scan(&r.Seq, &r.Evento, &r.Ator, &r.ConsultaID, &dados, &anterior, &hash, &r.CriadoEm); err != nil {
			return nil, fmt.Errorf("ler registro de auditoria: %w", err)
		}
		if canon, err := canonico(dados); err == nil {
			calculado, err := calcular(anterior, tenantID, r.Seq, r.Evento, r.Ator, r.ConsultaID, canon)
			r.HashOK = err == nil && bytes.Equal(calculado, hash)
		}
		itens = append(itens, r)
	}
	return itens, linhas.Err()
}

// canonico normaliza o JSON (chaves em ordem, números preservados) para o hash não depender de como o
// jsonb do Postgres reescreve o texto.
func canonico(bruto []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(bruto))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("normalizar dados da auditoria: %w", err)
	}
	return json.Marshal(v)
}

func calcular(anterior []byte, tenantID string, seq int64, evento, ator string, consultaID *string, dados []byte) ([]byte, error) {
	conteudo, err := json.Marshal(struct {
		Tenant     string          `json:"tenant_id"`
		Seq        int64           `json:"seq"`
		Evento     string          `json:"evento"`
		Ator       string          `json:"ator"`
		ConsultaID *string         `json:"consulta_id"`
		Dados      json.RawMessage `json:"dados"`
	}{tenantID, seq, evento, ator, consultaID, dados})
	if err != nil {
		return nil, fmt.Errorf("serializar registro de auditoria: %w", err)
	}
	h := sha256.New()
	h.Write(anterior)
	h.Write(conteudo)
	return h.Sum(nil), nil
}
