package bureau

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/documento"
)

// Documentos-fixture (ver README.md). Dígitos verificadores válidos, valores óbvios de ficção.
const (
	CPFFalecido        = "00000000191"
	CPFIrregular       = "00000000272"
	CPFScoreBaixo      = "00000000353"
	CPFScoreAlto       = "00000000434"
	CPFNomeConhecido   = "00000000515" // nome no bureau: NomeFixture; mande outro sobrenome para ver nome_divergente
	CPFIndisponivel    = "00000000604"
	CPFLento           = "00000000787"
	CPFCamposAusentes  = "00000000868"
	CNPJInativo        = "EXEMPLO0000154"
	CNPJRegular        = "EXEMPLO0000235"
	NomeFixture        = "JOANA PEREIRA DOS SANTOS"
	NascimentoFixture  = "1985-04-12"
	atrasoLento        = 30 * time.Second // sempre maior que o timeout de 2s da orquestração
	scoreMaximo        = 1000
	rendaBaseCentavos  = 150_000
	rendaFaixaCentavos = 2_000_000
)

func renda(centavos int64) *int64 { return &centavos }

var fixtures = map[string]Resultado{
	CPFFalecido:       {Score: 0, Nome: "JOSE FICTICIO DE TESTE", Nascimento: "1940-01-01", Situacao: Falecido},
	CPFIrregular:      {Score: 410, Restricoes: 2, DividaTotal: 350_000, Nome: "ANA FICTICIA IRREGULAR", Nascimento: "1990-06-15", Situacao: Irregular, Renda: renda(300_000)},
	CPFScoreBaixo:     {Score: 150, Restricoes: 5, Protestos: 2, DividaTotal: 4_200_000, Nome: "CARLOS FICTICIO BAIXO", Nascimento: "1978-11-30", Situacao: Regular, Renda: renda(180_000)},
	CPFScoreAlto:      {Score: 920, Nome: "BEATRIZ FICTICIA ALTA", Nascimento: "1982-03-08", Situacao: Regular, Renda: renda(1_200_000)},
	CPFNomeConhecido:  {Score: 680, Nome: NomeFixture, Nascimento: NascimentoFixture, Situacao: Regular, Renda: renda(450_000)},
	CPFCamposAusentes: {Score: 540, Situacao: Regular},
	CNPJInativo:       {Score: 200, Protestos: 3, DividaTotal: 9_000_000, Nome: "EMPRESA FICTICIA INATIVA LTDA", Situacao: Inativo},
	CNPJRegular:       {Score: 810, Nome: "EMPRESA FICTICIA EXEMPLO LTDA", Situacao: Regular, Renda: renda(25_000_000)},
}

type simulado struct{ atraso time.Duration }

// NovoSimulado devolve um bureau falso, determinístico pelo documento, que espera `atraso` antes de responder.
func NovoSimulado(atraso time.Duration) Bureau { return simulado{atraso: atraso} }

func (s simulado) Consultar(ctx context.Context, doc string) (Resultado, error) {
	doc, tipo, err := documento.Normalizar(doc)
	if err != nil {
		return Resultado{}, fmt.Errorf("bureau simulado: %w", err)
	}
	espera := s.atraso
	if doc == CPFLento {
		espera = atrasoLento
	}
	if err := esperar(ctx, espera); err != nil {
		return Resultado{}, indisponivel(err)
	}
	if doc == CPFIndisponivel {
		return Resultado{}, indisponivel(errors.New("fixture de indisponibilidade"))
	}
	r, ok := fixtures[doc]
	if !ok {
		r = derivado(doc, tipo)
	}
	r.Fontes = Fontes{Cadastro: FonteSimulado, Score: FonteSimulado, Restricoes: FonteSimulado}
	return r, nil
}

func esperar(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// derivado gera um perfil estável a partir do SHA-256 do documento, com situações e campos ausentes variados.
func derivado(doc string, tipo documento.Tipo) Resultado {
	h := sha256.Sum256([]byte(doc))
	n := func(i int) int { return int(binary.BigEndian.Uint16(h[2*i:])) }

	r := Resultado{
		Score:       n(0) % (scoreMaximo + 1),
		Restricoes:  n(1) % 4,
		Protestos:   n(2) % 3,
		DividaTotal: int64(n(3)%500) * 10_000,
		Situacao:    situacaoDerivada(n(4)%10, tipo),
	}
	if n(5)%4 != 0 { // 1 em 4 sem nome
		r.Nome = fmt.Sprintf("PESSOA SIMULADA %X", h[12:14])
		if tipo == documento.CNPJ {
			r.Nome = fmt.Sprintf("EMPRESA SIMULADA %X LTDA", h[12:14])
		}
	}
	if tipo == documento.CPF && n(6)%5 != 0 { // 1 em 5 sem nascimento
		r.Nascimento = fmt.Sprintf("%04d-%02d-%02d", 1940+n(7)%65, 1+n(8)%12, 1+n(9)%28)
	}
	if n(10)%3 != 0 { // 1 em 3 sem renda
		r.Renda = renda(rendaBaseCentavos + int64(n(11))*rendaFaixaCentavos/0xFFFF)
	}
	return r
}

// situacaoDerivada: 80% regular; o resto é a situação ruim que faz sentido para o tipo.
func situacaoDerivada(sorteio int, tipo documento.Tipo) Situacao {
	switch {
	case sorteio < 8:
		return Regular
	case tipo == documento.CNPJ:
		return Inativo
	case sorteio == 8:
		return Irregular
	default:
		return Falecido
	}
}
