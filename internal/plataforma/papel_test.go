package plataforma

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romeroalmeida/motor-credito/internal/plataforma/bancoteste"
)

// O boot só aceita um usuário sujeito à RLS (achado 8).
func TestConferirPapelDaApp(t *testing.T) {
	b := bancoteste.Novo(t)
	ctx := context.Background()
	for _, sql := range []string{
		`CREATE ROLE com_bypass LOGIN PASSWORD 'x' BYPASSRLS IN ROLE motor_app`,
		`CREATE ROLE herda_dono LOGIN PASSWORD 'x' IN ROLE dono`,
	} {
		if _, err := b.Dono.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	comoUsuario := func(nome string) *pgxpool.Pool {
		p, err := pgxpool.New(ctx, strings.Replace(b.URLApp, "app_teste:app_teste@", nome+":x@", 1))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Close)
		return p
	}
	casos := []struct {
		nome string
		pool *pgxpool.Pool
		erro string
	}{
		{"membro de motor_app", b.App, ""},
		{"dono/superusuário", b.Dono, "superusuário"},
		{"BYPASSRLS", comoUsuario("com_bypass"), "BYPASSRLS"},
		{"herda o dono das tabelas", comoUsuario("herda_dono"), "dono"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			err := ConferirPapelDaApp(ctx, c.pool)
			if c.erro == "" && err != nil {
				t.Fatalf("recusou usuário válido: %v", err)
			}
			if c.erro != "" && (err == nil || !strings.Contains(err.Error(), c.erro)) {
				t.Fatalf("erro = %v; quer conter %q", err, c.erro)
			}
		})
	}
}
