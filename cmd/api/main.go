// Comando api sobe o motor de crédito: aplica as migrations (com o usuário dono, se configurado) e serve
// a API pública e o painel com o usuário da aplicação (motor_app, sujeito à RLS).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/bureau"
	"github.com/romeroalmeida/motor-credito/internal/bureau/brasilapi"
	"github.com/romeroalmeida/motor-credito/internal/consulta"
	"github.com/romeroalmeida/motor-credito/internal/httpapi"
	"github.com/romeroalmeida/motor-credito/internal/painelauth"
	"github.com/romeroalmeida/motor-credito/internal/plataforma"
	"github.com/romeroalmeida/motor-credito/internal/plataforma/migrar"
	"github.com/romeroalmeida/motor-credito/internal/tenant"
	"github.com/romeroalmeida/motor-credito/migrations"
)

// atrasoBureau simula a latência de um bureau real.
const atrasoBureau = 50 * time.Millisecond

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := rodar(); err != nil {
		slog.Error("api encerrada com erro", "erro", err)
		os.Exit(1)
	}
}

func rodar() error {
	cfg, err := plataforma.Carregar()
	if err != nil {
		return err
	}
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	if err := migrarBanco(ctx, cfg.DatabaseURLMigracao); err != nil {
		return err
	}
	fonte, err := montarBureau(os.Getenv("BUREAU_CNPJ"))
	if err != nil {
		return err
	}
	pool, err := plataforma.NovoPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := plataforma.ConferirPapelDaApp(ctx, pool); err != nil {
		return err
	}

	tenants := tenant.NovoServico(pool, cfg.ChaveCifraDoc)
	auth := painelauth.Novo(pool, tenants, painelauth.Opcoes{
		ChaveCifra:     cfg.ChaveCifraDoc,
		CookieInseguro: painelauth.CookieInseguroPermitido(os.Getenv("COOKIE_INSEGURO_DEV"), cfg.Ambiente),
		URLPainel:      os.Getenv("URL_PAINEL"),
	})
	srv := &http.Server{
		Addr: ":" + cfg.Porta,
		Handler: httpapi.Novo(httpapi.Deps{
			Pool: pool, Tenants: tenants, Auth: auth, ProxiesConfiaveis: cfg.ProxiesConfiaveis,
			Consultas: consulta.Novo(pool, fonte, cfg.ChaveHMACDoc, cfg.ChaveCifraDoc),
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	erros := make(chan error, 1)
	go func() {
		slog.Info("api no ar", "porta", cfg.Porta, "ambiente", cfg.Ambiente)
		erros <- srv.ListenAndServe()
	}()
	select {
	case err := <-erros:
		return err
	case <-ctx.Done():
	}
	desligar, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	if err := srv.Shutdown(desligar); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("desligar: %w", err)
	}
	return nil
}

// montarBureau lê BUREAU_CNPJ: vazio ou "simulado" = só o simulado; "brasilapi" = cadastro real de CNPJ na
// BrasilAPI e o resto do simulado. Valor desconhecido impede subir (um erro de digitação não troca a fonte calado).
func montarBureau(cnpj string) (bureau.Bureau, error) {
	simulado := bureau.NovoSimulado(atrasoBureau)
	switch strings.TrimSpace(cnpj) {
	case "", "simulado":
		return simulado, nil
	case "brasilapi":
		return bureau.NovoComposto(brasilapi.Novo(""), simulado), nil
	}
	return nil, fmt.Errorf("BUREAU_CNPJ=%q desconhecido (use simulado ou brasilapi)", cnpj)
}

// migrarBanco usa o dono das tabelas só para migrar e fecha a conexão; vazio = não migra.
func migrarBanco(ctx context.Context, url string) error {
	if url == "" {
		slog.Info("DATABASE_URL_MIGRACAO vazia: migrations não aplicadas ao subir")
		return nil
	}
	dono, err := plataforma.NovoPool(ctx, url)
	if err != nil {
		return fmt.Errorf("migrar: %w", err)
	}
	defer dono.Close()
	aplicadas, err := migrar.Aplicar(ctx, dono, migrations.FS)
	if err != nil {
		return err
	}
	slog.Info("migrations", "aplicadas", aplicadas)
	return nil
}
