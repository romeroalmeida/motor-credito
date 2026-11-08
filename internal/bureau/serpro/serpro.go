// Package serpro consulta a situação cadastral do CPF na Receita pela API Consulta CPF v3 do Serpro
// (https://apicenter.estaleiro.serpro.gov.br/documentacao/consulta-cpf). Só cadastro: situação, nome e
// nascimento. Score e restrições não existem aqui (ver bureau.NovoComposto). O Serpro exige o nascimento.
package serpro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/bureau"
	"github.com/romeroalmeida/motor-credito/internal/documento"
)

// Ambiente de testes público do Serpro: CPFs fictícios, cota mínima. A chave é pública na documentação.
const (
	URLTrial   = "https://gateway.apiserpro.serpro.gov.br/consulta-cpf-df-trial/v3"
	ChaveTrial = "06aef429-a981-3ec5-a1f8-71d38d86481e"
)

const (
	limiteCorpo   = 64 << 10 // a resposta real tem ~150 bytes
	formatoSerpro = "02012006"
	idadeMinima   = 18 // o 422 do Serpro quer dizer "menor de 18"
)

type cliente struct {
	base, chave, fonte string
	http               *http.Client
	agora              func() time.Time
}

// Novo devolve o adaptador. trial só muda o nome da fonte na resposta (serpro-trial em vez de serpro);
// URL e chave vêm sempre de quem chama. O timeout vem do ctx de cada consulta.
func Novo(base, chave string, trial bool) bureau.Bureau {
	fonte := bureau.FonteSerpro
	if trial {
		fonte = bureau.FonteSerproTrial
	}
	return cliente{base: strings.TrimRight(base, "/"), chave: chave, fonte: fonte, http: &http.Client{}, agora: time.Now}
}

type resposta struct {
	Situacao *struct {
		Codigo string `json:"codigo"`
	} `json:"situacao"`
	Nome       string `json:"nome"`
	Nascimento string `json:"nascimento"` // DDMMAAAA
}

// Consultar sem nascimento não chama o Serpro: ele exige o dado.
func (c cliente) Consultar(ctx context.Context, doc string) (bureau.Resultado, error) {
	return c.ConsultarComNascimento(ctx, doc, "")
}

// ConsultarComNascimento consulta o CPF; nascimento em AAAA-MM-DD.
func (c cliente) ConsultarComNascimento(ctx context.Context, doc, nascimento string) (bureau.Resultado, error) {
	doc, tipo, err := documento.Normalizar(doc)
	if err != nil {
		return bureau.Resultado{}, fmt.Errorf("serpro: %w", err)
	}
	if tipo != documento.CPF {
		return bureau.Resultado{}, errors.New("serpro: só consulta CPF")
	}
	nasc, err := time.Parse(time.DateOnly, nascimento)
	if err != nil {
		return bureau.Resultado{}, errors.New("serpro: nascimento AAAA-MM-DD obrigatório")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/cpf/"+doc+"/"+nasc.Format(formatoSerpro), nil)
	if err != nil {
		return bureau.Resultado{}, indisponivel(errors.New("montar requisição"))
	}
	req.Header.Set("Authorization", "Bearer "+c.chave)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // url.Error traz a URL com o CPF, e o erro vai para o log
		}
		return bureau.Resultado{}, indisponivel(err)
	}
	defer resp.Body.Close()

	base := bureau.Resultado{Fontes: bureau.Fontes{Cadastro: c.fonte}}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusPartialContent: // 206: conteúdo parcial; o que faltar fica ausente
	case http.StatusUnavailableForLegalReasons: // 451: menor de 16, o Serpro não devolve dado nenhum
		base.Situacao = bureau.Inexistente
		return base, nil
	case http.StatusUnprocessableEntity: // 422: menor de 18
		if idade(nasc, c.agora()) >= idadeMinima {
			base.Situacao = bureau.Inexistente // o nascimento informado diz adulto e a Receita diz menor: não confere
			return base, nil
		}
		base.Nascimento = nascimento // o corte MENOR_IDADE da política pega
		return base, nil
	default:
		// 429 (cota), 401/403 (chave), 400, 404, 5xx: fail-closed.
		return bureau.Resultado{}, indisponivel(fmt.Errorf("status %d", resp.StatusCode))
	}

	var r resposta
	if err := json.NewDecoder(io.LimitReader(resp.Body, limiteCorpo)).Decode(&r); err != nil {
		return bureau.Resultado{}, indisponivel(fmt.Errorf("json: %w", err))
	}
	if r.Situacao == nil && resp.StatusCode == http.StatusOK {
		return bureau.Resultado{}, indisponivel(errors.New("situação ausente"))
	}
	if r.Situacao != nil {
		s, ok := situacoes[strings.TrimSpace(r.Situacao.Codigo)]
		if !ok {
			return bureau.Resultado{}, indisponivel(fmt.Errorf("situação desconhecida %q", r.Situacao.Codigo))
		}
		base.Situacao = s
	}
	if r.Nascimento != "" {
		d, err := time.Parse(formatoSerpro, r.Nascimento)
		if err != nil {
			return bureau.Resultado{}, indisponivel(errors.New("nascimento malformado"))
		}
		base.Nascimento = d.Format(time.DateOnly)
	}
	base.Nome = strings.TrimSpace(r.Nome)
	return base, nil
}

// situacoes: códigos de situação cadastral do CPF na Receita. Fora daqui vira indisponível (não decidimos no escuro).
var situacoes = map[string]bureau.Situacao{
	"0": bureau.Regular,     // regular
	"2": bureau.Irregular,   // suspensa
	"3": bureau.Falecido,    // titular falecido
	"4": bureau.Irregular,   // pendente de regularização
	"5": bureau.Inexistente, // cancelada por multiplicidade
	"8": bureau.Inexistente, // nula
	"9": bureau.Inexistente, // cancelada de ofício
}

func idade(n, agora time.Time) int {
	anos := agora.Year() - n.Year()
	if agora.Month() < n.Month() || (agora.Month() == n.Month() && agora.Day() < n.Day()) {
		anos--
	}
	return anos
}

func indisponivel(causa error) error {
	return fmt.Errorf("serpro: %w: %w", bureau.ErrIndisponivel, causa)
}
