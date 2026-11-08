package consulta

import (
	"net/mail"
	"net/netip"
	"time"
	"unicode/utf8"

	"github.com/romeroalmeida/motor-credito/internal/bureau"
	"github.com/romeroalmeida/motor-credito/internal/dinheiro"
	"github.com/romeroalmeida/motor-credito/internal/politica"
)

// Status além dos do motor (aprovado, negado, contraproposta).
const (
	StatusNegado            = string(politica.Negado)
	StatusDocumentoInvalido = "documento_invalido"
)

// Motivos produzidos pela orquestração (os da política vêm do motor).
const (
	MotivoDocumentoInvalido      = "DOCUMENTO_INVALIDO"
	MotivoBureauIndisponivel     = "BUREAU_INDISPONIVEL"
	MotivoAntifraudeIndisponivel = "ANTIFRAUDE_INDISPONIVEL"
	MotivoPoliticaIndisponivel   = "POLITICA_INDISPONIVEL"
	MotivoErroPolitica           = "ERRO_POLITICA"
)

// Finalidade e base legal (LGPD) aceitas na v1.
const (
	FinalidadeConcessao = "concessao_credito"
	BaseLegalCredito    = "protecao_credito_art7_x" // #nosec G101 -- base legal da LGPD, não é credencial
)

const (
	maxNome          = 200
	maxTelefone      = 30
	maxDispositivo   = 200
	formatoData      = "2006-01-02"
	maxEmail         = 254
	maxDocumentoBrut = 40
)

// Pedido é o crédito pedido: valor em centavos, prazo em meses. Opcional: sem ele a consulta é de limite.
type Pedido struct {
	Valor int64 `json:"valor"`
	Prazo int   `json:"prazo"`
}

// Dados são os opcionais; cada um liga mais checagens antifraude. "" / nil = não enviado.
type Dados struct {
	Nome       string `json:"nome"`
	Nascimento string `json:"nascimento"`
	Renda      *int64 `json:"renda"`
	Email      string `json:"email"`
	Telefone   string `json:"telefone"`
	IP         string `json:"ip"`
	DeviceID   string `json:"device_id"`
}

// Requisicao é o corpo de POST /v1/consultas.
type Requisicao struct {
	Documento  string  `json:"documento"`
	Finalidade string  `json:"finalidade"`
	Pedido     *Pedido `json:"pedido"` // nil = consulta de limite (pré-aprovação)
	Dados      Dados   `json:"dados"`
}

// Validar devolve campo → problema (400). Dígito verificador errado NÃO é erro aqui: vira documento_invalido.
func (r Requisicao) Validar() map[string]string {
	campos := map[string]string{}
	if r.Documento == "" || len(r.Documento) > maxDocumentoBrut {
		campos["documento"] = "obrigatório, até 40 caracteres"
	}
	if r.Finalidade != FinalidadeConcessao {
		campos["finalidade"] = "deve ser concessao_credito"
	}
	if r.Pedido != nil { // pedido parcial cai aqui: o campo que faltou vem zerado e é recusado
		validarPedido(*r.Pedido, campos)
	}
	d := r.Dados
	if utf8.RuneCountInString(d.Nome) > maxNome {
		campos["dados.nome"] = "até 200 caracteres"
	}
	if d.Nascimento != "" {
		if _, err := time.Parse(formatoData, d.Nascimento); err != nil {
			campos["dados.nascimento"] = "data no formato AAAA-MM-DD"
		}
	}
	if d.Renda != nil && *d.Renda < 0 {
		campos["dados.renda"] = "não pode ser negativa"
	}
	if d.Email != "" {
		if a, err := mail.ParseAddress(d.Email); err != nil || a.Address != d.Email || len(d.Email) > maxEmail {
			campos["dados.email"] = "e-mail inválido"
		}
	}
	if utf8.RuneCountInString(d.Telefone) > maxTelefone {
		campos["dados.telefone"] = "até 30 caracteres"
	}
	if d.IP != "" {
		if _, err := netip.ParseAddr(d.IP); err != nil {
			campos["dados.ip"] = "IPv4 ou IPv6 inválido"
		}
	}
	if utf8.RuneCountInString(d.DeviceID) > maxDispositivo {
		campos["dados.device_id"] = "até 200 caracteres"
	}
	return campos
}

func validarPedido(p Pedido, campos map[string]string) {
	if p.Valor < 1 || p.Valor > dinheiro.ValorMaximo {
		campos["pedido.valor"] = "entre 1 e 10000000000000 centavos"
	}
	if p.Prazo < 1 || p.Prazo > politica.PrazoMaximoMeses {
		campos["pedido.prazo"] = "entre 1 e 420 meses"
	}
}

// Antifraude é o bloco antifraude da resposta.
type Antifraude struct {
	Risco      int      `json:"risco"`
	Sinais     []string `json:"sinais"`
	Executadas []string `json:"checagens_executadas"`
	Puladas    []string `json:"checagens_puladas"`
}

// PoliticaRef é a versão da política que decidiu.
type PoliticaRef struct {
	ID     string `json:"id"`
	Versao int    `json:"versao"`
}

// Resposta é o contrato ConsultaResposta do OpenAPI.
type Resposta struct {
	ID         string            `json:"id"`
	Status     string            `json:"status"`
	Score      *int              `json:"score"`
	FaixaRisco *string           `json:"faixa_risco"`
	Oferta     *politica.Oferta  `json:"oferta"`
	Limite     *politica.Limite  `json:"limite"` // nil quando negado ou documento inválido
	Pedido     *Pedido           `json:"pedido"` // nil na consulta de limite
	Motivos    []politica.Motivo `json:"motivos"`
	Antifraude *Antifraude       `json:"antifraude"`
	Politica   *PoliticaRef      `json:"politica"`
	CriadoEm   time.Time         `json:"criado_em"`
	// De onde vieram os dados; nil quando o bureau não respondeu ou o documento é inválido.
	Fontes       *bureau.Fontes `json:"fontes"`
	ScoreFatores []bureau.Fator `json:"score_fatores"` // só com score cadastral
	Cadastro     *Cadastro      `json:"cadastro"`
}

// Cadastro é o contrato Cadastro: situação sempre; razão social e abertura só para CNPJ.
type Cadastro struct {
	Situacao    string `json:"situacao"`
	Abertura    string `json:"abertura,omitempty"`
	RazaoSocial string `json:"razao_social,omitempty"`
}

// registro é o que vai para consultas.decisao: a resposta mais a trilha de regras.
type registro struct {
	Resposta
	Trilha []politica.RegraAvaliada `json:"trilha"`
}

// ResultadoTeste é o contrato ResultadoTeste (teste de política no painel; nada é gravado).
type ResultadoTeste struct {
	Status       string                   `json:"status"`
	Score        *int                     `json:"score"`
	FaixaRisco   *string                  `json:"faixa_risco"`
	Oferta       *politica.Oferta         `json:"oferta"`
	Limite       *politica.Limite         `json:"limite"`
	Motivos      []politica.Motivo        `json:"motivos"`
	Antifraude   *Antifraude              `json:"antifraude"`
	Trilha       []politica.RegraAvaliada `json:"trilha"`
	Fontes       *bureau.Fontes           `json:"fontes"`
	ScoreFatores []bureau.Fator           `json:"score_fatores"`
	Cadastro     *Cadastro                `json:"cadastro"`
}

// Resumo é o contrato ConsultaResumo (lista do painel).
type Resumo struct {
	ID                 string           `json:"id"`
	DocumentoMascarado string           `json:"documento_mascarado"`
	TipoDocumento      *string          `json:"tipo_documento"`
	Status             string           `json:"status"`
	Score              *int             `json:"score"`
	FaixaRisco         *string          `json:"faixa_risco"`
	Pedido             *Pedido          `json:"pedido"`
	Oferta             *politica.Oferta `json:"oferta"`
	AntifraudeRisco    *int             `json:"antifraude_risco"`
	PoliticaVersao     *int             `json:"politica_versao"`
	CriadoEm           time.Time        `json:"criado_em"`
}

// Detalhe é o contrato ConsultaDetalhe.
type Detalhe struct {
	Resposta
	DocumentoMascarado string                   `json:"documento_mascarado"`
	TipoDocumento      *string                  `json:"tipo_documento"`
	Finalidade         string                   `json:"finalidade"`
	BaseLegal          string                   `json:"base_legal"`
	DadosInformados    []string                 `json:"dados_informados"`
	Trilha             []politica.RegraAvaliada `json:"trilha"`
}

// entrada é o que vai para consultas.entrada: o pedido e QUAIS dados vieram, nunca os valores pessoais.
type entrada struct {
	Pedido          *Pedido  `json:"pedido"`
	DadosInformados []string `json:"dados_informados"`
}

func dadosInformados(d Dados) []string {
	lista := []string{}
	for _, c := range []struct {
		nome string
		veio bool
	}{
		{"nome", d.Nome != ""}, {"nascimento", d.Nascimento != ""}, {"renda", d.Renda != nil}, {"email", d.Email != ""},
		{"telefone", d.Telefone != ""}, {"ip", d.IP != ""}, {"device_id", d.DeviceID != ""},
	} {
		if c.veio {
			lista = append(lista, c.nome)
		}
	}
	return lista
}
