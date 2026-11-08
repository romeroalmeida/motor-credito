import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { renderizarPagina } from '@/tests/renderizar'
import { PoliticaPage } from './PoliticaPage'

async function abrirFaixas() {
  const usuario = userEvent.setup()
  renderizarPagina(<PoliticaPage />)
  await usuario.click(await screen.findByRole('tab', { name: 'Faixas' }))
  return usuario
}

const grupo = (nome: string) => screen.getByRole('group', { name: nome })

describe('editor de política', () => {
  it('adiciona e remove condição na regra certa', async () => {
    const usuario = await abrirFaixas()
    const faixaA = grupo('Faixa A')
    expect(within(faixaA).getAllByRole('listitem', { name: /^Condição/ })).toHaveLength(1)

    await usuario.click(within(faixaA).getByRole('button', { name: 'Adicionar condição' }))
    expect(within(faixaA).getAllByRole('listitem', { name: /^Condição/ })).toHaveLength(2)
    expect(within(grupo('Faixa B')).getAllByRole('listitem', { name: /^Condição/ })).toHaveLength(2)

    await usuario.click(within(faixaA).getByRole('button', { name: 'Remover condição 1' }))
    const restantes = within(faixaA).getAllByRole('listitem', { name: /^Condição/ })
    expect(restantes).toHaveLength(1)
    // Sobrou a condição nova (score >= 0), não a original (score > 600).
    expect(within(faixaA).getByLabelText('Operador da condição 1')).toHaveValue('>=')
    expect(within(faixaA).getByLabelText('Valor da condição 1')).toHaveValue('0')
    expect(await screen.findByText('Alterações não salvas')).toBeInTheDocument()
  })

  it('publicar só chama o servidor depois da confirmação', async () => {
    const corpos: unknown[] = []
    server.use(
      http.post('*/painel/politicas/publicar', async ({ request }) => {
        corpos.push(await request.json())
        return HttpResponse.json(
          {
            versao: 4,
            publicada_por: 'Ana',
            publicada_em: '2026-11-07T15:00:00Z',
            revertida_de: null,
            conteudo: {},
          },
          { status: 201 },
        )
      }),
    )
    const usuario = userEvent.setup()
    renderizarPagina(<PoliticaPage />)

    await usuario.click(await screen.findByRole('button', { name: 'Publicar' }))
    const dialogo = await screen.findByRole('alertdialog', { name: 'Publicar como versão 4?' })
    expect(corpos).toHaveLength(0)

    await usuario.click(within(dialogo).getByRole('button', { name: 'Cancelar' }))
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(corpos).toHaveLength(0)

    await usuario.click(screen.getByRole('button', { name: 'Publicar' }))
    const deNovo = await screen.findByRole('alertdialog')
    await usuario.type(within(deNovo).getByLabelText('Comentário (opcional)'), 'taxa nova')
    await usuario.click(within(deNovo).getByRole('button', { name: 'Publicar agora' }))

    expect(await screen.findByText(/Versão 4 publicada/)).toBeInTheDocument()
    expect(corpos).toEqual([{ confirmacao: true, comentario: 'taxa nova' }])
  })

  it('erro de validação do servidor aparece na regra apontada, e só nela', async () => {
    server.use(
      http.post('*/painel/politicas/publicar', () =>
        HttpResponse.json(
          {
            erro: 'politica_invalida',
            detalhes: [{ regra: 'faixas[1].condicoes[0]', motivo: 'score comparado com texto' }],
          },
          { status: 422 },
        ),
      ),
    )
    const usuario = userEvent.setup()
    renderizarPagina(<PoliticaPage />)
    await usuario.click(await screen.findByRole('button', { name: 'Publicar' }))
    await usuario.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Publicar agora' }),
    )

    // Leva para a aba da regra com problema.
    expect(await screen.findByRole('tab', { name: /Faixas/, selected: true })).toBeInTheDocument()
    const faixaB = grupo('Faixa B')
    expect(within(faixaB).getByText('score comparado com texto')).toBeInTheDocument()
    expect(within(faixaB).getByLabelText('Valor da condição 1')).toHaveAttribute('aria-invalid', 'true')
    expect(within(faixaB).getByLabelText('Valor da condição 2')).toHaveAttribute('aria-invalid', 'false')
    expect(within(grupo('Faixa A')).queryByText('score comparado com texto')).not.toBeInTheDocument()
    expect(within(grupo('Faixa C')).queryByText('score comparado com texto')).not.toBeInTheDocument()
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  it('valor ilegível é barrado antes de ir ao servidor e marcado na faixa', async () => {
    const usuario = await abrirFaixas()
    const taxa = within(grupo('Faixa A')).getByLabelText('Taxa ao mês (%)')
    await usuario.clear(taxa)
    await usuario.type(taxa, 'abc')
    await usuario.click(screen.getByRole('button', { name: 'Salvar rascunho' }))
    expect(within(grupo('Faixa A')).getByText('Taxa inválida. Ex.: 2,90')).toBeInTheDocument()
    expect(taxa).toHaveAttribute('aria-invalid', 'true')
  })

  it('analista vê a política mas não edita', async () => {
    const usuario = userEvent.setup()
    renderizarPagina(<PoliticaPage />, { papel: 'analista' })
    await usuario.click(await screen.findByRole('tab', { name: 'Faixas' }))
    expect(screen.queryByRole('button', { name: 'Publicar' })).not.toBeInTheDocument()
    expect(within(grupo('Faixa A')).getByLabelText('Taxa ao mês (%)')).toBeDisabled()
    expect(screen.getByText(/só administradores editam/)).toBeInTheDocument()
  })
})
