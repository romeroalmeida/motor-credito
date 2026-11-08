// Comando semente cria o ambiente de demonstração: tenant, admin com TOTP, política modelo PF publicada e
// uma chave de API. Duas travas: SEED_DEMO=on E ambiente explicitamente desenvolvimento ou teste.
// Rodar de novo reaproveita o tenant e o admin, troca a senha e cria outra chave.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/argon2"

	"github.com/romeroalmeida/motor-credito/internal/plataforma"
	"github.com/romeroalmeida/motor-credito/internal/politica"
	"github.com/romeroalmeida/motor-credito/internal/repositorio"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
)

const (
	emailAdmin = "admin@demo.local"
	nomeTenant = "Demonstração"
)

func main() {
	if err := rodar(); err != nil {
		slog.Error("semente falhou", "erro", err)
		os.Exit(1)
	}
}

func rodar() error {
	cfg, err := plataforma.Carregar()
	if err != nil {
		return err
	}
	if !Permitida(os.Getenv("SEED_DEMO"), cfg) {
		slog.Warn("semente recusada: exige SEED_DEMO=on e AMBIENTE=desenvolvimento ou teste", "ambiente", cfg.Ambiente)
		os.Exit(2)
	}
	ctx := context.Background()
	pool, err := plataforma.NovoPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	d, err := semear(ctx, pool, cfg.ChaveCifraDoc)
	if err != nil {
		return err
	}
	fmt.Printf("tenant:        %s\n", d.tenantID)
	fmt.Printf("admin:         %s\n", emailAdmin)
	fmt.Printf("senha:         %s\n", d.senha)
	fmt.Printf("segredo TOTP:  %s\n", d.segredoTOTP)
	fmt.Printf("chave de API:  %s\n", d.chaveAPI)
	return nil
}

// demo é o que a semente imprime (segredos só no terminal de quem rodou).
type demo struct{ tenantID, senha, segredoTOTP, chaveAPI string }

// Permitida aplica as duas travas.
func Permitida(seedDemo string, cfg plataforma.Config) bool {
	return seedDemo == "on" && !cfg.EhProducao()
}

func semear(ctx context.Context, pool *pgxpool.Pool, chaveCifra []byte) (demo, error) {
	senha, err := aleatorio(18)
	if err != nil {
		return demo{}, err
	}
	hash, err := hashSenha(senha)
	if err != nil {
		return demo{}, err
	}
	usuarioID, tenantID, segredo, err := garantirAdmin(ctx, pool, chaveCifra, hash)
	if err != nil {
		return demo{}, err
	}
	repo := repositorio.ParaTenant(pool, tenantID)
	estado, err := repo.Politica(ctx)
	if err != nil {
		return demo{}, err
	}
	if estado.VersaoPublicada == nil {
		if _, err := repo.Publicar(ctx, politica.ModeloEmprestimoPF(), usuarioID, "modelo inicial (semente)", nil); err != nil {
			return demo{}, err
		}
	}
	chave, err := tenant.NovoServico(pool).CriarChaveComNome(ctx, tenantID, "demo")
	if err != nil {
		return demo{}, err
	}
	return demo{tenantID: tenantID, senha: senha, segredoTOTP: segredo, chaveAPI: chave.Segredo}, nil
}

// garantirAdmin cria tenant + admin + TOTP confirmado, ou reaproveita os existentes trocando a senha.
func garantirAdmin(ctx context.Context, pool *pgxpool.Pool, chaveCifra []byte, hash string) (usuarioID, tenantID, segredo string, err error) {
	var cifrado []byte
	err = pool.QueryRow(ctx, `SELECT u.id, a.tenant_id, t.segredo_cifrado FROM usuarios u
		JOIN totp t ON t.usuario_id = u.id, auth_associacoes(u.id) a WHERE lower(u.email) = $1 LIMIT 1`, emailAdmin).
		Scan(&usuarioID, &tenantID, &cifrado)
	if err == nil {
		claro, err := plataforma.Decifrar(chaveCifra, cifrado)
		if err != nil {
			return "", "", "", fmt.Errorf("decifrar TOTP do admin demo: %w", err)
		}
		_, err = pool.Exec(ctx, `UPDATE usuarios SET senha_hash = $2 WHERE id = $1`, usuarioID, hash)
		return usuarioID, tenantID, string(claro), err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", err
	}

	chave, err := totp.Generate(totp.GenerateOpts{Issuer: "Motor de Crédito", AccountName: emailAdmin})
	if err != nil {
		return "", "", "", err
	}
	segredo = chave.Secret()
	if cifrado, err = plataforma.Cifrar(chaveCifra, []byte(segredo)); err != nil {
		return "", "", "", err
	}
	usuarioID, tenantID = plataforma.NovoID("usr"), plataforma.NovoID("ten")
	err = tenant.EmTenant(ctx, pool, tenantID, func(tx pgx.Tx) error {
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO tenants (id, nome) VALUES ($1, $2)`, []any{tenantID, nomeTenant}},
			{`INSERT INTO usuarios (id, email, nome, senha_hash) VALUES ($1, $2, 'Admin Demo', $3)`, []any{usuarioID, emailAdmin, hash}},
			{`INSERT INTO totp (usuario_id, segredo_cifrado, confirmado_em) VALUES ($1, $2, now())`, []any{usuarioID, cifrado}},
			{`INSERT INTO associacoes (usuario_id, tenant_id, papel) VALUES ($1, $2, 'admin')`, []any{usuarioID, tenantID}},
		} {
			if _, err := tx.Exec(ctx, q.sql, q.args...); err != nil {
				return err
			}
		}
		return nil
	})
	return usuarioID, tenantID, segredo, err
}

func aleatorio(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashSenha repete o formato PHC do painelauth (argon2id, parâmetros OWASP), que o login sabe conferir.
func hashSenha(senha string) (string, error) {
	const memoriaKiB, iteracoes, vias, tamanho = 19 * 1024, 2, 1, 32
	sal := make([]byte, 16)
	if _, err := rand.Read(sal); err != nil {
		return "", err
	}
	h := argon2.IDKey([]byte(senha), sal, iteracoes, memoriaKiB, vias, tamanho)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, memoriaKiB, iteracoes, vias,
		b64.EncodeToString(sal), b64.EncodeToString(h)), nil
}
