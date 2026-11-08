package repositorio

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romeroalmeida/motor-credito/internal/plataforma"
)

// Contador implementa antifraude.Contador sobre antifraude_eventos, que não tem tenant_id: a velocidade
// é contada entre todos os clientes sem revelar quem consultou.
type Contador struct {
	pool  *pgxpool.Pool
	chave []byte // CHAVE_HMAC_DOC, também usada para o hash do dispositivo
}

// NovoContador recebe o pool da aplicação e a chave HMAC de documento.
func NovoContador(pool *pgxpool.Pool, chaveHMAC []byte) *Contador {
	return &Contador{pool: pool, chave: chaveHMAC}
}

// HashDispositivo é o HMAC do device_id; o id em claro nunca vai para o banco.
func HashDispositivo(chaveHMAC []byte, deviceID string) []byte {
	return plataforma.HashDoc(chaveHMAC, "dispositivo:"+deviceID)
}

// ConsultasDoc24h conta as consultas do documento nas últimas 24h, em todos os tenants.
func (c *Contador) ConsultasDoc24h(ctx context.Context, docHash []byte) (int, error) {
	var n int
	err := c.pool.QueryRow(ctx, `SELECT count(*) FROM antifraude_eventos
		WHERE doc_hash = $1 AND criado_em > now() - interval '24 hours'`, docHash).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("contar consultas do documento: %w", err)
	}
	return n, nil
}

// DocsPorDispositivo24h conta documentos distintos vistos no dispositivo nas últimas 24h.
func (c *Contador) DocsPorDispositivo24h(ctx context.Context, deviceID string) (int, error) {
	var n int
	err := c.pool.QueryRow(ctx, `SELECT count(DISTINCT doc_hash) FROM antifraude_eventos
		WHERE dispositivo_hash = $1 AND criado_em > now() - interval '24 hours'`, HashDispositivo(c.chave, deviceID)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("contar documentos do dispositivo: %w", err)
	}
	return n, nil
}
