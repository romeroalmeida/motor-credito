package repositorio

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/romeroalmeida/motor-credito/internal/auditoria"
	"github.com/romeroalmeida/motor-credito/internal/plataforma"
	"github.com/romeroalmeida/motor-credito/internal/politica"
)

// nomePolitica: a v1 tem uma política por tenant; a tabela já aceita várias por nome.
const nomePolitica = "principal"

// PoliticaEstado é o rascunho em edição mais a versão publicada.
type PoliticaEstado struct {
	ID                        string            `json:"id"`
	Nome                      string            `json:"nome"`
	Rascunho                  politica.Politica `json:"rascunho"`
	RascunhoAtualizadoEm      time.Time         `json:"rascunho_atualizado_em"`
	RascunhoAtualizadoPor     *string           `json:"rascunho_atualizado_por"` // nome do usuário
	VersaoPublicada           *int              `json:"versao_publicada"`
	RascunhoDifereDaPublicada bool              `json:"rascunho_difere_da_publicada"`
}

// Versao é uma versão publicada (imutável).
type Versao struct {
	Versao       int                `json:"versao"`
	PublicadaPor string             `json:"publicada_por"` // nome do usuário
	PublicadaEm  time.Time          `json:"publicada_em"`
	RevertidaDe  *int               `json:"revertida_de"`
	Comentario   string             `json:"comentario"`
	Conteudo     *politica.Politica `json:"conteudo,omitempty"` // nil na listagem
}

// Publicada é o que a consulta precisa para decidir.
type Publicada struct {
	ID       string
	Versao   int
	Conteudo politica.Politica
}

// Politica devolve o estado da política do tenant; na primeira vez cria o rascunho a partir do modelo PF.
func (r *Repo) Politica(ctx context.Context) (PoliticaEstado, error) {
	var e PoliticaEstado
	err := r.emTx(ctx, func(tx pgx.Tx) error {
		if err := garantirPolitica(ctx, tx, r.tenantID); err != nil {
			return err
		}
		return lerEstado(ctx, tx, r.tenantID, &e)
	})
	if err != nil {
		return PoliticaEstado{}, fmt.Errorf("ler política: %w", err)
	}
	return e, nil
}

// SalvarRascunho troca o rascunho (sem compilar: isso é feito ao testar e ao publicar).
func (r *Repo) SalvarRascunho(ctx context.Context, p politica.Politica, usuarioID string) (PoliticaEstado, error) {
	conteudo, err := json.Marshal(p)
	if err != nil {
		return PoliticaEstado{}, fmt.Errorf("serializar rascunho: %w", err)
	}
	var e PoliticaEstado
	err = r.emTx(ctx, func(tx pgx.Tx) error {
		if err := garantirPolitica(ctx, tx, r.tenantID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE politicas SET rascunho = $3::jsonb, rascunho_atualizado_em = now(),
			rascunho_atualizado_por = $4 WHERE tenant_id = $1 AND nome = $2`,
			r.tenantID, nomePolitica, string(conteudo), usuarioID); err != nil {
			return err
		}
		return lerEstado(ctx, tx, r.tenantID, &e)
	})
	if err != nil {
		return PoliticaEstado{}, fmt.Errorf("salvar rascunho: %w", err)
	}
	return e, nil
}

// Publicar grava conteudo como versão N+1 e a torna a publicada, com auditoria na mesma transação.
// Quem chama já compilou o conteudo (422 fica com a camada HTTP). revertidaDe != nil marca uma reversão.
func (r *Repo) Publicar(ctx context.Context, conteudo politica.Politica, usuarioID, comentario string, revertidaDe *int) (Versao, error) {
	bruto, err := json.Marshal(conteudo)
	if err != nil {
		return Versao{}, fmt.Errorf("serializar versão: %w", err)
	}
	var v Versao
	err = r.emTx(ctx, func(tx pgx.Tx) error {
		if err := garantirPolitica(ctx, tx, r.tenantID); err != nil {
			return err
		}
		var politicaID string
		// FOR UPDATE serializa publicações concorrentes do mesmo tenant.
		if err := tx.QueryRow(ctx, `SELECT id FROM politicas WHERE tenant_id = $1 AND nome = $2 FOR UPDATE`,
			r.tenantID, nomePolitica).Scan(&politicaID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(versao), 0) + 1 FROM politica_versoes
			WHERE politica_id = $1 AND tenant_id = $2`, politicaID, r.tenantID).Scan(&v.Versao); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO politica_versoes (politica_id, tenant_id, versao, conteudo, publicada_por, revertida_de, comentario)
			VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7)`, politicaID, r.tenantID, v.Versao, string(bruto), usuarioID, revertidaDe, comentario); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE politicas SET versao_publicada = $2 WHERE id = $1 AND tenant_id = $3`,
			politicaID, v.Versao, r.tenantID); err != nil {
			return err
		}
		if err := auditoria.Registrar(ctx, tx, r.tenantID, auditoria.EventoPoliticaPublicada, auditoria.AtorUsuario(usuarioID), nil,
			map[string]any{"politica_id": politicaID, "versao": v.Versao, "revertida_de": revertidaDe, "comentario": comentario}); err != nil {
			return err
		}
		return lerVersao(ctx, tx, r.tenantID, v.Versao, &v)
	})
	if err != nil {
		return Versao{}, fmt.Errorf("publicar política: %w", err)
	}
	return v, nil
}

// Versoes lista o histórico, mais nova primeiro, sem o conteúdo.
func (r *Repo) Versoes(ctx context.Context) ([]Versao, error) {
	versoes := []Versao{}
	err := r.emTx(ctx, func(tx pgx.Tx) error {
		linhas, err := tx.Query(ctx, `SELECT v.versao, u.nome, v.publicada_em, v.revertida_de, v.comentario
			FROM politica_versoes v JOIN politicas p ON p.id = v.politica_id JOIN usuarios u ON u.id = v.publicada_por
			WHERE v.tenant_id = $1 AND p.nome = $2 ORDER BY v.versao DESC`, r.tenantID, nomePolitica)
		if err != nil {
			return err
		}
		versoes, err = pgx.CollectRows(linhas, func(l pgx.CollectableRow) (Versao, error) {
			var v Versao
			err := l.Scan(&v.Versao, &v.PublicadaPor, &v.PublicadaEm, &v.RevertidaDe, &v.Comentario)
			v.PublicadaEm = v.PublicadaEm.UTC()
			return v, err
		})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("listar versões: %w", err)
	}
	return versoes, nil
}

// Versao devolve uma versão com conteúdo; inexistente ou de outro tenant → ErrNaoEncontrado.
func (r *Repo) Versao(ctx context.Context, n int) (Versao, error) {
	var v Versao
	err := r.emTx(ctx, func(tx pgx.Tx) error { return lerVersao(ctx, tx, r.tenantID, n, &v) })
	if err != nil {
		return Versao{}, naoEncontrado(err)
	}
	return v, nil
}

// PoliticaPublicada devolve a versão em vigor; nunca publicada → ErrNaoEncontrado.
func (r *Repo) PoliticaPublicada(ctx context.Context) (Publicada, error) {
	var p Publicada
	var conteudo []byte
	err := r.emTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT p.id, v.versao, v.conteudo FROM politicas p
			JOIN politica_versoes v ON v.politica_id = p.id AND v.versao = p.versao_publicada
			WHERE p.tenant_id = $1 AND p.nome = $2`, r.tenantID, nomePolitica).Scan(&p.ID, &p.Versao, &conteudo)
	})
	if err != nil {
		return Publicada{}, naoEncontrado(err)
	}
	if err := json.Unmarshal(conteudo, &p.Conteudo); err != nil {
		return Publicada{}, fmt.Errorf("ler versão publicada: %w", err)
	}
	return p, nil
}

func garantirPolitica(ctx context.Context, tx pgx.Tx, tenantID string) error {
	modelo, err := json.Marshal(politica.ModeloEmprestimoPF())
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO politicas (id, tenant_id, nome, rascunho) VALUES ($1, $2, $3, $4::jsonb)
		ON CONFLICT (tenant_id, nome) DO NOTHING`, plataforma.NovoID("pol"), tenantID, nomePolitica, string(modelo))
	return err
}

func lerEstado(ctx context.Context, tx pgx.Tx, tenantID string, e *PoliticaEstado) error {
	var rascunho []byte
	err := tx.QueryRow(ctx, `SELECT p.id, p.nome, p.rascunho, p.rascunho_atualizado_em, u.nome, p.versao_publicada,
			p.versao_publicada IS NOT NULL AND p.rascunho IS DISTINCT FROM v.conteudo
		FROM politicas p
		LEFT JOIN usuarios u ON u.id = p.rascunho_atualizado_por
		LEFT JOIN politica_versoes v ON v.politica_id = p.id AND v.versao = p.versao_publicada
		WHERE p.tenant_id = $1 AND p.nome = $2`, tenantID, nomePolitica).
		Scan(&e.ID, &e.Nome, &rascunho, &e.RascunhoAtualizadoEm, &e.RascunhoAtualizadoPor, &e.VersaoPublicada, &e.RascunhoDifereDaPublicada)
	if err != nil {
		return err
	}
	e.RascunhoAtualizadoEm = e.RascunhoAtualizadoEm.UTC()
	return json.Unmarshal(rascunho, &e.Rascunho)
}

func lerVersao(ctx context.Context, tx pgx.Tx, tenantID string, n int, v *Versao) error {
	var conteudo []byte
	err := tx.QueryRow(ctx, `SELECT v.versao, u.nome, v.publicada_em, v.revertida_de, v.comentario, v.conteudo
		FROM politica_versoes v JOIN politicas p ON p.id = v.politica_id JOIN usuarios u ON u.id = v.publicada_por
		WHERE v.tenant_id = $1 AND p.nome = $2 AND v.versao = $3`, tenantID, nomePolitica, n).
		Scan(&v.Versao, &v.PublicadaPor, &v.PublicadaEm, &v.RevertidaDe, &v.Comentario, &conteudo)
	if err != nil {
		return err
	}
	v.PublicadaEm = v.PublicadaEm.UTC()
	v.Conteudo = &politica.Politica{}
	return json.Unmarshal(conteudo, v.Conteudo)
}
