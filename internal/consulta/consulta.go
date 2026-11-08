// Package consulta orquestra uma decisão de crédito (desenho §3): documento → bureau → antifraude →
// política publicada → motor → grava consulta + auditoria numa transação.
//
// Regra de ouro: qualquer falha no caminho vira "negado" (ou documento_invalido), nunca "aprovado".
package consulta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romeroalmeida/motor-credito/internal/antifraude"
	"github.com/romeroalmeida/motor-credito/internal/bureau"
	"github.com/romeroalmeida/motor-credito/internal/documento"
	"github.com/romeroalmeida/motor-credito/internal/plataforma"
	"github.com/romeroalmeida/motor-credito/internal/politica"
	"github.com/romeroalmeida/motor-credito/internal/repositorio"
)

// TempoMaximoFontes é o tempo total para bureau + antifraude (desenho §3).
const TempoMaximoFontes = 2 * time.Second

// scoreMaximo é o topo da escala de score do bureau (0..1000, OpenAPI).
const scoreMaximo = 1000

// Servico decide e consulta decisões.
type Servico struct {
	pool       *pgxpool.Pool
	bureau     bureau.Bureau
	contador   antifraude.Contador
	chaveHMAC  []byte
	chaveCifra []byte
	timeout    time.Duration
	agora      func() time.Time
	// shortcut: cache sem limite por id+versão (versões são poucas e imutáveis); trocar por LRU se crescer.
	compiladas sync.Map
}

// Novo monta o serviço com o pool da aplicação, o bureau e as chaves de documento.
func Novo(pool *pgxpool.Pool, b bureau.Bureau, chaveHMAC, chaveCifra []byte) *Servico {
	return &Servico{
		pool: pool, bureau: b, contador: repositorio.NovoContador(pool, chaveHMAC),
		chaveHMAC: chaveHMAC, chaveCifra: chaveCifra, timeout: TempoMaximoFontes, agora: time.Now,
	}
}

// fontePolitica entrega a política a aplicar e a referência da versão (nil no teste do painel).
type fontePolitica func(ctx context.Context) (*PoliticaRef, *politica.Compilada, error)

// avaliacao é o resultado da orquestração antes de gravar.
type avaliacao struct {
	reg  registro
	doc  string         // normalizado; "" se inválido
	tipo documento.Tipo // "" se inválido
}

// avaliar roda o fluxo inteiro sem gravar nada. Começa negado: só o motor, sem erro, muda o status.
func (s *Servico) avaliar(ctx context.Context, req Requisicao, fonte fontePolitica) avaliacao {
	av := avaliacao{reg: registro{
		Resposta: Resposta{Status: StatusNegado, Pedido: req.Pedido, Motivos: []politica.Motivo{}},
		Trilha:   []politica.RegraAvaliada{},
	}}
	doc, tipo, err := documento.Normalizar(req.Documento)
	if err != nil {
		return documentoInvalido(av)
	}
	av.doc, av.tipo = doc, tipo

	ctxFontes, cancelar := context.WithTimeout(ctx, s.timeout)
	defer cancelar()
	b, err := s.bureau.Consultar(ctxFontes, doc)
	if errors.Is(err, documento.ErrInvalido) {
		return documentoInvalido(av)
	}
	if err != nil {
		slog.WarnContext(ctx, "bureau falhou", "documento", documento.Mascarar(doc), "erro", err)
		return negar(av, MotivoBureauIndisponivel, "Bureau de crédito indisponível; tente novamente")
	}
	if b.Situacao == bureau.Falecido {
		return documentoInvalido(av)
	}
	if b.Score < 0 || b.Score > scoreMaximo { // resposta corrompida: não decide com ela
		slog.WarnContext(ctx, "bureau devolveu score fora da faixa", "documento", documento.Mascarar(doc), "score", b.Score)
		return negar(av, MotivoBureauIndisponivel, "Bureau de crédito indisponível; tente novamente")
	}
	score := b.Score
	av.reg.Score = &score

	docHash := plataforma.HashDoc(s.chaveHMAC, doc)
	af, err := antifraude.Avaliar(ctxFontes, antifraude.Entrada{
		DocHash: docHash, Tipo: string(tipo), Nome: req.Dados.Nome, Nascimento: req.Dados.Nascimento,
		Email: req.Dados.Email, IP: req.Dados.IP, DeviceID: req.Dados.DeviceID, Bureau: b,
	}, s.contador)
	if err != nil {
		slog.WarnContext(ctx, "antifraude falhou", "documento", documento.Mascarar(doc), "erro", err)
		return negar(av, MotivoAntifraudeIndisponivel, "Checagens antifraude indisponíveis; tente novamente")
	}
	av.reg.Antifraude = &Antifraude{Risco: af.Risco, Sinais: af.Sinais, Executadas: af.Executadas, Puladas: af.Puladas}

	ref, compilada, err := fonte(ctx)
	av.reg.Politica = ref
	if err != nil {
		slog.ErrorContext(ctx, "política indisponível", "erro", err)
		return negar(av, MotivoPoliticaIndisponivel, "Nenhuma política de crédito publicada")
	}
	d, err := compilada.Avaliar(s.contexto(req, b, af))
	if err != nil {
		slog.ErrorContext(ctx, "avaliar política", "politica", ref, "erro", err)
		return negar(av, MotivoErroPolitica, "Falha ao avaliar a política de crédito")
	}
	av.reg.Status = string(d.Status)
	if d.FaixaRisco != "" {
		av.reg.FaixaRisco = &d.FaixaRisco
	}
	av.reg.Oferta, av.reg.Motivos, av.reg.Trilha = d.Oferta, d.Motivos, d.Trilha
	return av
}

// contexto monta o que as regras enxergam. Renda: a informada vale sobre a do bureau.
// Idade: o nascimento válido do bureau vale sobre o informado (o cliente pode errar ou mentir).
func (s *Servico) contexto(req Requisicao, b bureau.Resultado, af antifraude.Resultado) politica.Contexto {
	c := politica.Contexto{
		Score: b.Score, Restricoes: b.Restricoes, Protestos: b.Protestos, DividaTotal: b.DividaTotal,
		Renda: req.Dados.Renda, PedidoValor: req.Pedido.Valor, PedidoPrazo: req.Pedido.Prazo,
		FraudeRisco: af.Risco, FraudeSinais: af.Sinais,
	}
	if c.Renda == nil {
		c.Renda = b.Renda
	}
	// Bureau ausente ou inválido (formato, data impossível, futuro) cai para o informado; o válido sempre vale.
	idade, ok := idadeEm(b.Nascimento, s.agora())
	if !ok {
		idade, ok = idadeEm(req.Dados.Nascimento, s.agora())
	}
	if ok {
		c.Idade = &idade
	}
	return c
}

func idadeEm(nascimento string, agora time.Time) (int, bool) {
	n, err := time.Parse(formatoData, nascimento)
	if err != nil || n.After(agora) {
		return 0, false
	}
	idade := agora.Year() - n.Year()
	if agora.Month() < n.Month() || (agora.Month() == n.Month() && agora.Day() < n.Day()) {
		idade--
	}
	return idade, true
}

func negar(av avaliacao, codigo, texto string) avaliacao {
	av.reg.Status, av.reg.Oferta, av.reg.FaixaRisco = StatusNegado, nil, nil
	av.reg.Motivos = append(av.reg.Motivos, politica.Motivo{Codigo: codigo, Texto: texto})
	return av
}

// documentoInvalido cobre dígito errado e pessoa falecida, sem dizer qual dos dois (desenho §5).
func documentoInvalido(av avaliacao) avaliacao {
	av.reg.Status, av.reg.Score, av.reg.Oferta, av.reg.FaixaRisco = StatusDocumentoInvalido, nil, nil, nil
	av.reg.Motivos = []politica.Motivo{{Codigo: MotivoDocumentoInvalido, Texto: "Documento inválido para concessão de crédito"}}
	return av
}

// Decidir roda a consulta e grava. ator é "api:<chave>"; idemKey vazio = sem idempotência.
// Com idemKey já gravada por outra requisição, devolve a resposta dela sem gravar de novo.
func (s *Servico) Decidir(ctx context.Context, tenantID, ator, idemKey string, req Requisicao) (Resposta, error) {
	repo := repositorio.ParaTenant(s.pool, tenantID)
	av := s.avaliar(ctx, req, s.politicaPublicada(repo))
	av.reg.ID = plataforma.NovoID("cons")
	av.reg.CriadoEm = s.agora().UTC().Truncate(time.Microsecond) // precisão do timestamptz

	nova, err := s.linha(av, req, idemKey)
	if err != nil {
		return Resposta{}, err
	}
	existente, err := repo.GravarConsulta(ctx, nova, ator)
	if err != nil {
		return Resposta{}, err
	}
	if existente != nil {
		var reg registro
		if err := json.Unmarshal(existente, &reg); err != nil {
			return Resposta{}, fmt.Errorf("ler decisão idempotente: %w", err)
		}
		return reg.Resposta, nil
	}
	return av.reg.Resposta, nil
}

func (s *Servico) linha(av avaliacao, req Requisicao, idemKey string) (repositorio.NovaConsulta, error) {
	decisao, err := json.Marshal(av.reg)
	if err != nil {
		return repositorio.NovaConsulta{}, fmt.Errorf("serializar decisão: %w", err)
	}
	ent, err := json.Marshal(entrada{Pedido: req.Pedido, DadosInformados: dadosInformados(req.Dados)})
	if err != nil {
		return repositorio.NovaConsulta{}, fmt.Errorf("serializar entrada: %w", err)
	}
	n := repositorio.NovaConsulta{
		ID: av.reg.ID, Finalidade: req.Finalidade, BaseLegal: BaseLegalCredito, Entrada: ent, Status: av.reg.Status,
		Score: av.reg.Score, FaixaRisco: av.reg.FaixaRisco, Decisao: decisao, CriadoEm: av.reg.CriadoEm,
	}
	if av.doc != "" {
		n.DocHash = plataforma.HashDoc(s.chaveHMAC, av.doc)
		if n.DocCifrado, err = plataforma.Cifrar(s.chaveCifra, []byte(av.doc)); err != nil {
			return repositorio.NovaConsulta{}, fmt.Errorf("cifrar documento: %w", err)
		}
		tipo := string(av.tipo)
		n.DocTipo = &tipo
		if req.Dados.DeviceID != "" {
			n.DispositivoHash = repositorio.HashDispositivo(s.chaveHMAC, req.Dados.DeviceID)
		}
	}
	if av.reg.Antifraude != nil {
		n.AntifraudeRisco = &av.reg.Antifraude.Risco
	}
	if av.reg.Politica != nil {
		n.PoliticaID, n.PoliticaVersao = &av.reg.Politica.ID, &av.reg.Politica.Versao
	}
	if idemKey != "" {
		n.IdempotencyKey = &idemKey
	}
	return n, nil
}

// politicaPublicada lê a versão em vigor e compila uma vez por id+versão.
func (s *Servico) politicaPublicada(repo *repositorio.Repo) fontePolitica {
	return func(ctx context.Context) (*PoliticaRef, *politica.Compilada, error) {
		pub, err := repo.PoliticaPublicada(ctx)
		if err != nil {
			return nil, nil, err
		}
		ref := &PoliticaRef{ID: pub.ID, Versao: pub.Versao}
		chave := pub.ID + ":" + strconv.Itoa(pub.Versao)
		if c, ok := s.compiladas.Load(chave); ok {
			return ref, c.(*politica.Compilada), nil
		}
		c, err := politica.Compilar(pub.Conteudo)
		if err != nil {
			return ref, nil, err
		}
		s.compiladas.Store(chave, c)
		return ref, c, nil
	}
}

// Simular roda a política compilada contra um documento sem gravar consulta (botão "testar" do painel).
func (s *Servico) Simular(ctx context.Context, req Requisicao, c *politica.Compilada) ResultadoTeste {
	av := s.avaliar(ctx, req, func(context.Context) (*PoliticaRef, *politica.Compilada, error) { return nil, c, nil })
	r := av.reg
	return ResultadoTeste{Status: r.Status, Score: r.Score, FaixaRisco: r.FaixaRisco, Oferta: r.Oferta,
		Motivos: r.Motivos, Antifraude: r.Antifraude, Trilha: r.Trilha}
}

// ValidarSimulacao valida o pedido e os dados do teste (a finalidade é fixa).
func ValidarSimulacao(req Requisicao) map[string]string {
	req.Finalidade = FinalidadeConcessao
	return req.Validar()
}

// HashDocumento normaliza e devolve o HMAC (busca por documento completo no painel).
func (s *Servico) HashDocumento(entrada string) ([]byte, error) {
	doc, _, err := documento.Normalizar(entrada)
	if err != nil {
		return nil, err
	}
	return plataforma.HashDoc(s.chaveHMAC, doc), nil
}
