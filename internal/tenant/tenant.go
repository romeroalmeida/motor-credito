// Package tenant cuida do cliente (tenant), das chaves de API, da autenticação da API pública e da idempotência.
package tenant

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// MaxIPsPermitidos é o limite da lista de IPs do tenant (contrato OpenAPI).
const MaxIPsPermitidos = 50

const prefixoChave = "ck_live_"

var (
	// ErrNaoEncontrado cobre recurso inexistente e recurso de outro tenant (sempre 404).
	ErrNaoEncontrado = errors.New("não encontrado")
	// ErrIPInvalido indica item da lista de IPs que não é IP nem CIDR.
	ErrIPInvalido = errors.New("IP ou CIDR inválido")
)

// Tenant é o cliente da plataforma.
type Tenant struct {
	ID, Nome        string
	LimitePorMinuto int
	IPsPermitidos   []netip.Prefix // vazio = qualquer IP
}

// Chave é uma chave de API sem o segredo.
type Chave struct {
	ID, Nome    string
	CriadaEm    time.Time
	RevogadaEm  *time.Time
	UltimoUsoEm *time.Time
}

// Prefixo é a forma exibível da chave: "ck_live_<id>_…" (sem o segredo).
func (c Chave) Prefixo() string { return prefixoChave + c.ID + "_…" }

// ChaveCriada traz o segredo completo "ck_live_<id>_<segredo>"; ele aparece só aqui.
type ChaveCriada struct{ ID, Segredo string }

// Servico acessa o Postgres sempre com app.tenant_id definido (RLS) e filtro explícito de tenant.
type Servico struct {
	pool      *pgxpool.Pool
	agora     func() time.Time
	limitador *Limitador
}

// NovoServico recebe o pool do usuário da aplicação (membro de motor_app).
func NovoServico(pool *pgxpool.Pool) *Servico {
	return &Servico{pool: pool, agora: time.Now, limitador: NovoLimitador()}
}

// EmTenant roda fn numa transação com SET LOCAL app.tenant_id, que liga a RLS para esse tenant.
func EmTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string, fn func(pgx.Tx) error) error {
	if tenantID == "" {
		return errors.New("tenant vazio")
	}
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenantID); err != nil {
			return fmt.Errorf("definir tenant: %w", err)
		}
		return fn(tx)
	})
}

// Buscar devolve o tenant com limite e IPs permitidos.
func (s *Servico) Buscar(ctx context.Context, tenantID string) (Tenant, error) {
	var t Tenant
	err := EmTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id, nome, limite_por_minuto, ips_permitidos FROM tenants WHERE id = $1`,
			tenantID).Scan(&t.ID, &t.Nome, &t.LimitePorMinuto, &t.IPsPermitidos)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, ErrNaoEncontrado
	}
	if err != nil {
		return Tenant{}, fmt.Errorf("buscar tenant: %w", err)
	}
	return t, nil
}

// CriarChave cria uma chave sem nome. Ver CriarChaveComNome.
func (s *Servico) CriarChave(ctx context.Context, tenantID string) (ChaveCriada, error) {
	return s.CriarChaveComNome(ctx, tenantID, "")
}

// CriarChaveComNome gera id e segredo; o banco guarda só o SHA-256 do segredo.
func (s *Servico) CriarChaveComNome(ctx context.Context, tenantID, nome string) (ChaveCriada, error) {
	var criada ChaveCriada
	err := EmTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var err error
		criada, err = inserirChave(ctx, tx, tenantID, nome)
		return err
	})
	if err != nil {
		return ChaveCriada{}, fmt.Errorf("criar chave: %w", err)
	}
	return criada, nil
}

func inserirChave(ctx context.Context, tx pgx.Tx, tenantID, nome string) (ChaveCriada, error) {
	id := ulid.Make().String()
	bruto := make([]byte, 32)
	if _, err := rand.Read(bruto); err != nil {
		return ChaveCriada{}, fmt.Errorf("gerar segredo: %w", err)
	}
	segredo := hex.EncodeToString(bruto)
	hash := sha256.Sum256([]byte(segredo))
	if _, err := tx.Exec(ctx, `INSERT INTO chaves_api (id, tenant_id, segredo_hash, nome) VALUES ($1, $2, $3, $4)`,
		id, tenantID, hash[:], nome); err != nil {
		return ChaveCriada{}, err
	}
	return ChaveCriada{ID: id, Segredo: prefixoChave + id + "_" + segredo}, nil
}

// RevogarChave é idempotente; chave inexistente ou de outro tenant → ErrNaoEncontrado.
func (s *Servico) RevogarChave(ctx context.Context, tenantID, chaveID string) error {
	err := EmTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE chaves_api SET revogada_em = coalesce(revogada_em, now())
			WHERE id = $1 AND tenant_id = $2`, chaveID, tenantID)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrNaoEncontrado
		}
		return err
	})
	if err != nil && !errors.Is(err, ErrNaoEncontrado) {
		return fmt.Errorf("revogar chave: %w", err)
	}
	return err
}

// GirarChave cria uma chave com o mesmo nome e revoga a antiga, na mesma transação.
// Chave inexistente, já revogada ou de outro tenant → ErrNaoEncontrado.
func (s *Servico) GirarChave(ctx context.Context, tenantID, chaveID string) (ChaveCriada, error) {
	var criada ChaveCriada
	err := EmTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		var nome string
		err := tx.QueryRow(ctx, `UPDATE chaves_api SET revogada_em = now()
			WHERE id = $1 AND tenant_id = $2 AND revogada_em IS NULL RETURNING nome`, chaveID, tenantID).Scan(&nome)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNaoEncontrado
		}
		if err != nil {
			return err
		}
		criada, err = inserirChave(ctx, tx, tenantID, nome)
		return err
	})
	if err != nil && !errors.Is(err, ErrNaoEncontrado) {
		return ChaveCriada{}, fmt.Errorf("girar chave: %w", err)
	}
	return criada, err
}

// ListarChaves devolve as chaves do tenant, mais novas primeiro, sem segredo.
func (s *Servico) ListarChaves(ctx context.Context, tenantID string) ([]Chave, error) {
	chaves := []Chave{}
	err := EmTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		linhas, err := tx.Query(ctx, `SELECT id, nome, criada_em, revogada_em, ultimo_uso_em FROM chaves_api
			WHERE tenant_id = $1 ORDER BY criada_em DESC, id DESC`, tenantID)
		if err != nil {
			return err
		}
		chaves, err = pgx.CollectRows(linhas, func(l pgx.CollectableRow) (Chave, error) {
			var c Chave
			err := l.Scan(&c.ID, &c.Nome, &c.CriadaEm, &c.RevogadaEm, &c.UltimoUsoEm)
			return c, err
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("listar chaves: %w", err)
	}
	return chaves, nil
}

// DefinirIPs troca a lista de IPs permitidos. Aceita IP solto (vira /32 ou /128) ou CIDR.
func (s *Servico) DefinirIPs(ctx context.Context, tenantID string, ips []string) ([]netip.Prefix, error) {
	prefixos, err := ParsearIPs(ips)
	if err != nil {
		return nil, err
	}
	err = EmTenant(ctx, s.pool, tenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE tenants SET ips_permitidos = $2 WHERE id = $1`, tenantID, prefixos)
		if err == nil && tag.RowsAffected() == 0 {
			return ErrNaoEncontrado
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("definir IPs: %w", err)
	}
	return prefixos, nil
}

// ParsearIPs valida e normaliza a lista (máximo MaxIPsPermitidos).
func ParsearIPs(ips []string) ([]netip.Prefix, error) {
	if len(ips) > MaxIPsPermitidos {
		return nil, fmt.Errorf("%w: no máximo %d itens", ErrIPInvalido, MaxIPsPermitidos)
	}
	prefixos := make([]netip.Prefix, 0, len(ips))
	for _, ip := range ips {
		p, err := netip.ParsePrefix(ip)
		if err != nil {
			a, errA := netip.ParseAddr(ip)
			if errA != nil {
				return nil, fmt.Errorf("%w: %q", ErrIPInvalido, ip)
			}
			p = netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen())
		}
		prefixos = append(prefixos, p.Masked())
	}
	return prefixos, nil
}
