// Package brasilapi busca o cadastro público de CNPJ na BrasilAPI (https://brasilapi.com.br/docs#tag/CNPJ).
// Só cadastro: nome, situação, abertura e os dados públicos que alimentam o score cadastral (bureau.Empresa).
// Restrições não existem aqui (ver bureau.NovoComposto).
package brasilapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/romeroalmeida/motor-credito/internal/bureau"
	"github.com/romeroalmeida/motor-credito/internal/documento"
)

const (
	URLPadrao   = "https://brasilapi.com.br/api/cnpj/v1/"
	userAgent   = "motor-credito/1.0 (+consulta cadastral de CNPJ)"
	limiteCorpo = 1 << 20 // 1 MB; a resposta real tem ~10 KB
)

type cliente struct {
	base string
	http *http.Client
}

// Novo devolve o adaptador; base "" usa URLPadrao. O timeout vem do ctx de cada consulta.
func Novo(base string) bureau.Bureau {
	if base == "" {
		base = URLPadrao
	}
	return cliente{base: strings.TrimRight(base, "/") + "/", http: &http.Client{}}
}

type resposta struct {
	RazaoSocial         string            `json:"razao_social"`
	DescricaoSituacao   string            `json:"descricao_situacao_cadastral"`
	DataInicioAtividade string            `json:"data_inicio_atividade"`
	CapitalSocial       json.Number       `json:"capital_social"` // reais, às vezes com centavos
	CodigoPorte         int               `json:"codigo_porte"`
	OpcaoSimples        bool              `json:"opcao_pelo_simples"`
	OpcaoMEI            bool              `json:"opcao_pelo_mei"`
	CodigoNatureza      int               `json:"codigo_natureza_juridica"`
	QSA                 []json.RawMessage `json:"qsa"` // só contamos; os dados dos sócios não são lidos
}

func (c cliente) Consultar(ctx context.Context, doc string) (bureau.Resultado, error) {
	doc, tipo, err := documento.Normalizar(doc)
	if err != nil {
		return bureau.Resultado{}, fmt.Errorf("brasilapi: %w", err)
	}
	if tipo != documento.CNPJ {
		return bureau.Resultado{}, errors.New("brasilapi: só consulta CNPJ")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+doc, nil)
	if err != nil {
		return bureau.Resultado{}, indisponivel(err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // url.Error traz a URL com o CNPJ, e o erro vai para o log
		}
		return bureau.Resultado{}, indisponivel(err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return bureau.Resultado{Situacao: bureau.Inexistente}, nil
	case resp.StatusCode != http.StatusOK:
		// 429, 5xx e também 400 (ex.: CNPJ alfanumérico que a BrasilAPI ainda não aceita): fail-closed.
		return bureau.Resultado{}, indisponivel(fmt.Errorf("status %d", resp.StatusCode))
	}

	var r resposta
	if err := json.NewDecoder(io.LimitReader(resp.Body, limiteCorpo)).Decode(&r); err != nil {
		return bureau.Resultado{}, indisponivel(fmt.Errorf("json: %w", err))
	}
	situacao, ok := situacoes[strings.ToUpper(strings.TrimSpace(r.DescricaoSituacao))]
	if !ok {
		return bureau.Resultado{}, indisponivel(fmt.Errorf("situação desconhecida %q", r.DescricaoSituacao))
	}
	if r.DataInicioAtividade != "" {
		if _, err := time.Parse(time.DateOnly, r.DataInicioAtividade); err != nil {
			return bureau.Resultado{}, indisponivel(fmt.Errorf("data_inicio_atividade: %w", err))
		}
	}
	capital, err := centavos(r.CapitalSocial)
	if err != nil {
		return bureau.Resultado{}, indisponivel(fmt.Errorf("capital_social: %w", err))
	}
	return bureau.Resultado{
		Nome: strings.TrimSpace(r.RazaoSocial), Situacao: situacao, Abertura: r.DataInicioAtividade,
		Empresa: &bureau.Empresa{
			CapitalSocial: capital, CodigoPorte: r.CodigoPorte, Simples: r.OpcaoSimples, MEI: r.OpcaoMEI,
			Natureza: r.CodigoNatureza, Socios: len(r.QSA),
		},
		Fontes: bureau.Fontes{Cadastro: bureau.FonteBrasilAPI},
	}, nil
}

// centavos converte o capital em reais ("1500", "1500.5", "1500.55") sem float; "" (ausente/null) = 0.
// Casas além dos centavos são truncadas. Negativo, expoente ou estouro de int64 = erro (fail-closed).
func centavos(n json.Number) (int64, error) {
	s := n.String()
	if s == "" {
		return 0, nil
	}
	inteiro, frac, _ := strings.Cut(s, ".")
	frac = (frac + "00")[:2]
	r, err1 := strconv.ParseUint(inteiro, 10, 63)
	c, err2 := strconv.ParseUint(frac, 10, 8)
	if err1 != nil || err2 != nil || strings.ContainsAny(s, "eE+-") || r > (1<<63-1-99)/100 {
		return 0, fmt.Errorf("valor %q fora do formato", s)
	}
	return int64(r*100 + c), nil
}

// situacoes: descricao_situacao_cadastral da Receita. Fora daqui vira indisponível (não decidimos no escuro).
var situacoes = map[string]bureau.Situacao{
	"ATIVA":    bureau.Regular,
	"BAIXADA":  bureau.Inativo,
	"INAPTA":   bureau.Inativo,
	"SUSPENSA": bureau.Inativo,
	"NULA":     bureau.Inativo,
}

func indisponivel(causa error) error {
	return fmt.Errorf("brasilapi: %w: %w", bureau.ErrIndisponivel, causa)
}
