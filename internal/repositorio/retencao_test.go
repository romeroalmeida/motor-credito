package repositorio_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/auditoria"
	"github.com/romeroalmeida/motor-credito/internal/plataforma/bancoteste"
	"github.com/romeroalmeida/motor-credito/internal/repositorio"
)

func TestAplicarRetencao(t *testing.T) {
	b := bancoteste.Novo(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := b.Dono.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	agora := time.Date(2031, 11, 7, 12, 0, 0, 0, time.UTC)
	const dias = 30
	corte := agora.AddDate(0, 0, -dias)

	exec(`INSERT INTO tenants (id, nome) VALUES ('ten_a', 'A'), ('ten_b', 'B'), ('ten_c', 'C')`)
	inserir := func(id, tenantID string, criado time.Time) {
		exec(`INSERT INTO consultas (id, tenant_id, doc_hash, doc_cifrado, doc_tipo, finalidade, base_legal, entrada,
				status, score, faixa_risco, decisao, idempotency_key, idempotency_hash, criado_em)
			VALUES ($1, $2, '\x01', '\x02', 'cpf', 'concessao_credito', 'protecao_credito_art7_x',
				'{"pedido":{"valor":100000,"prazo":24}}', 'aprovado', 920, 'A', '{"status":"aprovado"}', $1, '\x03', $3)`,
			id, tenantID, criado)
	}
	inserir("cons_velha_a", "ten_a", corte.Add(-time.Microsecond)) // um µs antes do corte: vencida
	inserir("cons_borda_a", "ten_a", corte)                        // exatamente no corte: ainda dentro do prazo
	inserir("cons_nova_b", "ten_b", agora.Add(-time.Hour))
	exec(`INSERT INTO antifraude_eventos (doc_hash, criado_em) VALUES ('\x01', $1), ('\x01', $2)`, corte.Add(-time.Second), agora)

	// Cache de idempotência: a velha some, a nova e a da borda ficam; ten_c só tem cache vencido (as consultas dele
	// já teriam sido anonimizadas por uma execução anterior) e ainda assim precisa ser visitado.
	exec(`INSERT INTO idempotencia (tenant_id, chave, hash_corpo, resposta, criado_em) VALUES
		('ten_a', 'idem_velha', '\x01', '\x02', $1), ('ten_a', 'idem_borda', '\x01', '\x02', $2),
		('ten_b', 'idem_nova', '\x01', '\x02', $3), ('ten_c', 'idem_so_cache', '\x01', '\x02', $1)`,
		corte.Add(-time.Microsecond), corte, agora)

	n, err := repositorio.AplicarRetencao(ctx, b.App, dias, agora)
	if err != nil || n != 1 {
		t.Fatalf("AplicarRetencao = %d, %v; quer 1", n, err)
	}
	type linha struct {
		hash, cifrado, idemHash []byte
		idem                    *string
		apagado                 *time.Time
		status, faixa           string
		score                   int
		entrada                 string
	}
	ler := func(id string) linha {
		t.Helper()
		var l linha
		if err := b.Dono.QueryRow(ctx, `SELECT doc_hash, doc_cifrado, idempotency_hash, idempotency_key, dados_apagados_em,
				status, faixa_risco, score, entrada::text FROM consultas WHERE id = $1`, id).
			Scan(&l.hash, &l.cifrado, &l.idemHash, &l.idem, &l.apagado, &l.status, &l.faixa, &l.score, &l.entrada); err != nil {
			t.Fatal(err)
		}
		return l
	}
	velha := ler("cons_velha_a")
	if velha.hash != nil || velha.cifrado != nil || velha.idem != nil || velha.idemHash != nil || velha.apagado == nil {
		t.Fatalf("consulta vencida não foi anonimizada: %+v", velha)
	}
	if velha.status != "aprovado" || velha.faixa != "A" || velha.score != 920 || velha.entrada == "" {
		t.Fatalf("agregado perdido: %+v", velha)
	}
	for _, id := range []string{"cons_borda_a", "cons_nova_b"} {
		if l := ler(id); l.hash == nil || l.cifrado == nil || l.idem == nil || l.apagado != nil {
			t.Fatalf("%s mexida antes do prazo: %+v", id, l)
		}
	}
	var eventos int
	if err := b.Dono.QueryRow(ctx, `SELECT count(*) FROM antifraude_eventos`).Scan(&eventos); err != nil || eventos != 1 {
		t.Fatalf("eventos de velocidade = %d, %v; quer só o recente", eventos, err)
	}
	var chaves []string
	linhas, err := b.Dono.Query(ctx, `SELECT chave FROM idempotencia ORDER BY chave`)
	if err != nil {
		t.Fatal(err)
	}
	for linhas.Next() {
		var c string
		if err := linhas.Scan(&c); err != nil {
			t.Fatal(err)
		}
		chaves = append(chaves, c)
	}
	if err := linhas.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(chaves, ",") != "idem_borda,idem_nova" {
		t.Fatalf("idempotencia depois da retenção = %v; quer só idem_borda e idem_nova", chaves)
	}
	contarAuditoria := func(tenantID string) int {
		var c int
		if err := b.Dono.QueryRow(ctx, `SELECT count(*) FROM auditoria WHERE tenant_id = $1 AND evento = $2 AND ator = $3`,
			tenantID, auditoria.EventoRetencaoAplicada, repositorio.AtorRetencao).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	if contarAuditoria("ten_a") != 1 || contarAuditoria("ten_b") != 0 || contarAuditoria("ten_c") != 1 {
		t.Fatalf("auditoria: a=%d b=%d c=%d; quer 1, 0 e 1",
			contarAuditoria("ten_a"), contarAuditoria("ten_b"), contarAuditoria("ten_c"))
	}
	if err := auditoria.Verificar(ctx, b.Dono, "ten_a"); err != nil {
		t.Fatalf("cadeia da auditoria: %v", err)
	}

	// Idempotente: rodar de novo não apaga nada nem registra outro evento.
	if n, err := repositorio.AplicarRetencao(ctx, b.App, dias, agora); err != nil || n != 0 {
		t.Fatalf("segunda execução = %d, %v; quer 0", n, err)
	}
	if contarAuditoria("ten_a") != 1 {
		t.Fatalf("segunda execução registrou outro evento")
	}
	if _, err := repositorio.AplicarRetencao(ctx, b.App, 0, agora); err == nil {
		t.Fatalf("retenção de 0 dias aceita (apagaria tudo)")
	}
}
