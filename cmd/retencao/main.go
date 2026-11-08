// Comando retencao aplica a retenção LGPD (desenho §4): consultas mais velhas que RETENCAO_DIAS (padrão 1825,
// 5 anos) perdem o documento e a Idempotency-Key e ficam só com o agregado. Roda com o usuário da aplicação
// (DATABASE_URL, sujeito à RLS), é idempotente e foi feito para um agendador (cron ou o profile "retencao" do
// docker compose). Sai com código diferente de zero em qualquer erro, para o agendador alertar.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/plataforma"
	"github.com/romeroalmeida/motor-credito/internal/repositorio"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := rodar(); err != nil {
		slog.Error("retenção falhou", "erro", err)
		os.Exit(1)
	}
}

func rodar() error {
	dias, err := diasRetencao(os.Getenv("RETENCAO_DIAS"))
	if err != nil {
		return err
	}
	url := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if url == "" {
		return errors.New("DATABASE_URL ausente")
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancelar()
	pool, err := plataforma.NovoPool(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	n, err := repositorio.AplicarRetencao(ctx, pool, dias, time.Now())
	if err != nil {
		return err
	}
	slog.Info("retenção aplicada", "consultas_anonimizadas", n, "retencao_dias", dias)
	return nil
}

// diasRetencao: vazio = padrão; qualquer coisa que não seja inteiro ≥ 1 impede rodar (um erro de digitação não
// pode virar "apagar tudo" nem "não apagar nunca" sem ninguém ver).
func diasRetencao(v string) (int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return repositorio.RetencaoPadraoDias, nil
	}
	d, err := strconv.Atoi(v)
	if err != nil || d < 1 {
		return 0, fmt.Errorf("RETENCAO_DIAS=%q inválido: use um inteiro de dias ≥ 1", v)
	}
	return d, nil
}
