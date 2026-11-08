package consulta

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/romeroalmeida/motor-credito/internal/documento"
	"github.com/romeroalmeida/motor-credito/internal/plataforma"
	"github.com/romeroalmeida/motor-credito/internal/repositorio"
)

// Listar devolve uma página de consultas do tenant e o cursor da próxima (nil na última).
func (s *Servico) Listar(ctx context.Context, tenantID string, f repositorio.Filtro) ([]Resumo, *string, error) {
	limite := f.Limite
	f.Limite++ // um a mais para saber se há próxima página
	linhas, err := repositorio.ParaTenant(s.pool, tenantID).ListarConsultas(ctx, f)
	if err != nil {
		return nil, nil, err
	}
	var proximo *string
	if len(linhas) > limite {
		linhas = linhas[:limite]
		proximo = &linhas[limite-1].ID
	}
	itens := make([]Resumo, 0, len(linhas))
	for _, l := range linhas {
		var reg registro
		if err := json.Unmarshal(l.Decisao, &reg); err != nil {
			return nil, nil, fmt.Errorf("ler decisão %s: %w", l.ID, err)
		}
		itens = append(itens, Resumo{
			ID: l.ID, DocumentoMascarado: s.mascarar(ctx, l.ID, l.DocCifrado), TipoDocumento: l.DocTipo, Status: l.Status,
			Score: l.Score, FaixaRisco: l.FaixaRisco, Pedido: reg.Pedido, Oferta: reg.Oferta,
			AntifraudeRisco: l.AntifraudeRisco, PoliticaVersao: l.PoliticaVersao, CriadoEm: l.CriadoEm,
		})
	}
	return itens, proximo, nil
}

// Detalhe devolve a consulta com a trilha; de outro tenant → repositorio.ErrNaoEncontrado.
func (s *Servico) Detalhe(ctx context.Context, tenantID, id string) (Detalhe, error) {
	l, err := repositorio.ParaTenant(s.pool, tenantID).Consulta(ctx, id)
	if err != nil {
		return Detalhe{}, err
	}
	var reg registro
	if err := json.Unmarshal(l.Decisao, &reg); err != nil {
		return Detalhe{}, fmt.Errorf("ler decisão: %w", err)
	}
	var ent entrada
	if err := json.Unmarshal(l.Entrada, &ent); err != nil {
		return Detalhe{}, fmt.Errorf("ler entrada: %w", err)
	}
	if ent.DadosInformados == nil {
		ent.DadosInformados = []string{}
	}
	return Detalhe{
		Resposta: reg.Resposta, DocumentoMascarado: s.mascarar(ctx, l.ID, l.DocCifrado), TipoDocumento: l.DocTipo,
		Finalidade: l.Finalidade, BaseLegal: l.BaseLegal, DadosInformados: ent.DadosInformados, Trilha: reg.Trilha,
	}, nil
}

// mascarar decifra (AAD = id da consulta) só para mascarar. Documento inválido ou apagado pela retenção (nil),
// cifrado de outra linha, adulterado ou gravado antes da AAD (pilha local antiga) viram "***".
func (s *Servico) mascarar(ctx context.Context, id string, cifrado []byte) string {
	if cifrado == nil {
		return "***"
	}
	claro, err := plataforma.DecifrarAAD(s.chaveCifra, cifrado, []byte(id))
	if err != nil {
		slog.ErrorContext(ctx, "decifrar documento", "consulta", id, "erro", err)
		return "***"
	}
	return documento.Mascarar(string(claro))
}
