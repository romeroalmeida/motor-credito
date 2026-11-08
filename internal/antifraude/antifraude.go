// Package antifraude roda as checagens do desenho §6 e devolve risco e sinais. Informa; quem decide é a política.
package antifraude

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/bureau"
)

// Entrada traz o que foi enviado na consulta ("" = não enviado) e o resultado do bureau.
type Entrada struct {
	DocHash                               []byte
	Tipo                                  string // "cpf"|"cnpj"
	Nome, Nascimento, Email, IP, DeviceID string
	Bureau                                bureau.Resultado
	Referencia                            time.Time // data da consulta; zero = agora (testes injetam)
}

// Resultado: Risco 0..100, sinais disparados e as checagens executadas e puladas (por falta de dado).
type Resultado struct {
	Risco                       int
	Sinais, Executadas, Puladas []string
}

// Contador é implementado por E sobre o Postgres.
type Contador interface {
	ConsultasDoc24h(ctx context.Context, docHash []byte) (int, error) // em todos os tenants
	DocsPorDispositivo24h(ctx context.Context, deviceID string) (int, error)
}

// Sinais (enum do api/openapi.yaml).
const (
	SinalCPFIrregular             = "cpf_irregular"
	SinalCNPJInativo              = "cnpj_inativo"
	SinalVelocidadeAlta           = "velocidade_alta"
	SinalNomeDivergente           = "nome_divergente"
	SinalNascimentoDivergente     = "nascimento_divergente"
	SinalEmailDescartavel         = "email_descartavel"
	SinalIPEstrangeiro            = "ip_estrangeiro"
	SinalIPDatacenter             = "ip_datacenter"
	SinalDispositivoMultiplosCPFs = "dispositivo_multiplos_cpfs"
	SinalCNPJRecente              = "cnpj_recente"
)

// Pesos de cada sinal no risco.
const (
	PesoDocumentoIrregular   = 40
	PesoVelocidadeAlta       = 30
	PesoNomeDivergente       = 30
	PesoNascimentoDivergente = 30
	PesoEmailDescartavel     = 15
	PesoIPEstrangeiro        = 15
	PesoIPDatacenter         = 20
	PesoDispositivo          = 30
	PesoCNPJRecente          = 20
	RiscoMaximo              = 100
)

// Limites de velocidade: o sinal dispara quando a contagem das últimas 24h passa do limite.
const (
	LimiteConsultasDoc24h       = 5
	LimiteDocsPorDispositivo24h = 2
	// CNPJ com menos dias de abertura que isto dispara cnpj_recente.
	DiasCNPJRecente = 180
)

var pesos = map[string]int{
	SinalCPFIrregular:             PesoDocumentoIrregular,
	SinalCNPJInativo:              PesoDocumentoIrregular,
	SinalVelocidadeAlta:           PesoVelocidadeAlta,
	SinalNomeDivergente:           PesoNomeDivergente,
	SinalNascimentoDivergente:     PesoNascimentoDivergente,
	SinalEmailDescartavel:         PesoEmailDescartavel,
	SinalIPEstrangeiro:            PesoIPEstrangeiro,
	SinalIPDatacenter:             PesoIPDatacenter,
	SinalDispositivoMultiplosCPFs: PesoDispositivo,
	SinalCNPJRecente:              PesoCNPJRecente,
}

// checagem devolve executou=false quando falta o dado de que precisa.
type checagem struct {
	nome  string // enum Checagem do api/openapi.yaml
	rodar func(ctx context.Context, e Entrada, c Contador) (executou bool, sinais []string, err error)
}

var checagens = []checagem{
	{"documento", checarDocumento},
	{"velocidade", checarVelocidade},
	{"nome", checarNome},
	{"nascimento", checarNascimento},
	{"email", checarEmail},
	{"ip", checarIP},
	{"dispositivo", checarDispositivo},
}

// Avaliar roda todas as checagens. Erro do Contador aborta (fail-closed: E decide o que fazer).
func Avaliar(ctx context.Context, e Entrada, c Contador) (Resultado, error) {
	r := Resultado{Sinais: []string{}, Executadas: []string{}, Puladas: []string{}}
	for _, ch := range checagens {
		executou, sinais, err := ch.rodar(ctx, e, c)
		if err != nil {
			return Resultado{}, fmt.Errorf("antifraude: checagem %s: %w", ch.nome, err)
		}
		if !executou {
			r.Puladas = append(r.Puladas, ch.nome)
			continue
		}
		r.Executadas = append(r.Executadas, ch.nome)
		r.Sinais = append(r.Sinais, sinais...)
	}
	for _, s := range r.Sinais {
		r.Risco += pesos[s]
	}
	r.Risco = min(r.Risco, RiscoMaximo)
	return r, nil
}

// checarDocumento: irregular/inativo pontua; falecido e inexistente não (E transforma em documento_invalido).
// CNPJ aberto há menos de DiasCNPJRecente dias pontua cnpj_recente.
func checarDocumento(_ context.Context, e Entrada, _ Contador) (bool, []string, error) {
	s := e.Bureau.Situacao
	abertura, temAbertura := diaUTC(e.Bureau.Abertura)
	temAbertura = temAbertura && e.Tipo == "cnpj"
	if s == "" && !temAbertura {
		return false, nil, nil
	}
	var sinais []string
	switch {
	case s != bureau.Irregular && s != bureau.Inativo:
	case e.Tipo == "cnpj":
		sinais = append(sinais, SinalCNPJInativo)
	default:
		sinais = append(sinais, SinalCPFIrregular)
	}
	if temAbertura {
		ref := e.Referencia
		if ref.IsZero() {
			ref = time.Now()
		}
		hoje := time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, time.UTC)
		// abertura no futuro (dado ruim) também conta como recente: fail-closed
		if hoje.Sub(abertura) < DiasCNPJRecente*24*time.Hour {
			sinais = append(sinais, SinalCNPJRecente)
		}
	}
	return true, sinais, nil
}

// diaUTC lê AAAA-MM-DD; ok=false se vazio ou malformado.
func diaUTC(s string) (time.Time, bool) {
	d, err := time.Parse(time.DateOnly, strings.TrimSpace(s))
	return d, err == nil
}

func checarVelocidade(ctx context.Context, e Entrada, c Contador) (bool, []string, error) {
	if len(e.DocHash) == 0 || c == nil {
		return false, nil, nil
	}
	n, err := c.ConsultasDoc24h(ctx, e.DocHash)
	return acimaDoLimite(n, err, LimiteConsultasDoc24h, SinalVelocidadeAlta)
}

func checarDispositivo(ctx context.Context, e Entrada, c Contador) (bool, []string, error) {
	if e.DeviceID == "" || c == nil {
		return false, nil, nil
	}
	n, err := c.DocsPorDispositivo24h(ctx, e.DeviceID)
	return acimaDoLimite(n, err, LimiteDocsPorDispositivo24h, SinalDispositivoMultiplosCPFs)
}

func acimaDoLimite(n int, err error, limite int, sinal string) (bool, []string, error) {
	if err != nil {
		return false, nil, err
	}
	if n > limite {
		return true, []string{sinal}, nil
	}
	return true, nil, nil
}

func checarNome(_ context.Context, e Entrada, _ Contador) (bool, []string, error) {
	if strings.TrimSpace(e.Nome) == "" || strings.TrimSpace(e.Bureau.Nome) == "" {
		return false, nil, nil
	}
	if nomeDivergente(e.Nome, e.Bureau.Nome) {
		return true, []string{SinalNomeDivergente}, nil
	}
	return true, nil, nil
}

func checarNascimento(_ context.Context, e Entrada, _ Contador) (bool, []string, error) {
	informado, doBureau := strings.TrimSpace(e.Nascimento), strings.TrimSpace(e.Bureau.Nascimento)
	if informado == "" || doBureau == "" {
		return false, nil, nil
	}
	if informado != doBureau {
		return true, []string{SinalNascimentoDivergente}, nil
	}
	return true, nil, nil
}

func checarEmail(_ context.Context, e Entrada, _ Contador) (bool, []string, error) {
	arroba := strings.LastIndexByte(e.Email, '@')
	if arroba < 0 {
		return false, nil, nil
	}
	dominio := strings.ToLower(strings.TrimSpace(e.Email[arroba+1:]))
	if dominio == "" {
		return false, nil, nil
	}
	if _, ok := emailsDescartaveis[dominio]; ok {
		return true, []string{SinalEmailDescartavel}, nil
	}
	return true, nil, nil
}
