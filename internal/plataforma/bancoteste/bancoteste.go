// Package bancoteste sobe um Postgres 17 descartável (testcontainers) com todas as migrations aplicadas.
// Use só em testes de integração: bancoteste.Novo(t).
package bancoteste

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/romeroalmeida/motor-credito/internal/plataforma/migrar"
	"github.com/romeroalmeida/motor-credito/migrations"
)

// Banco é um Postgres com o schema aplicado.
type Banco struct {
	Dono            *pgxpool.Pool // dono das tabelas: não sofre RLS; use só para preparar dados
	App             *pgxpool.Pool // usuário membro de motor_app: é assim que a aplicação conecta
	URLDono, URLApp string
}

// Novo sobe o container, aplica as migrations, cria o usuário "app_teste" em motor_app e
// encerra tudo no fim do teste. Pula com -short.
func Novo(t testing.TB) *Banco {
	t.Helper()
	if testing.Short() {
		t.Skip("teste de integração: precisa de Docker (rode sem -short)")
	}
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("motor"),
		postgres.WithUsername("dono"),
		postgres.WithPassword("dono"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("subir postgres: %v", err)
	}
	urlDono, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("url do postgres: %v", err)
	}
	dono := abrir(t, urlDono)
	if _, err := migrar.Aplicar(ctx, dono, migrations.FS); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	if _, err := dono.Exec(ctx, `CREATE ROLE app_teste LOGIN PASSWORD 'app_teste' IN ROLE motor_app`); err != nil {
		t.Fatalf("criar usuário da aplicação: %v", err)
	}
	host, err := ctr.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	porta, err := ctr.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatal(err)
	}
	urlApp := "postgres://app_teste:app_teste@" + host + ":" + porta.Port() + "/motor?sslmode=disable"
	return &Banco{Dono: dono, App: abrir(t, urlApp), URLDono: urlDono, URLApp: urlApp}
}

func abrir(t testing.TB, url string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("abrir pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
