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

  it('testar sem valor e prazo consulta o limite e mostra Pré-aprovado com as simulações', async () => {
    let corpo: Record<string, unknown> | undefined
    server.events.on('request:start', async ({ request }) => {
      if (request.url.endsWith('/painel/politicas/testar')) corpo = await request.clone().json()
    })
    const usuario = userEvent.setup()
    renderizarPagina(<PoliticaPage />)
    await usuario.type(await screen.findByLabelText('CPF ou CNPJ'), '00000000434')
    await usuario.clear(screen.getByLabelText('Valor pedido (R$)'))
    await usuario.clear(screen.getByLabelText('Prazo (meses)'))
    await usuario.click(screen.getByRole('button', { name: 'Testar rascunho' }))

    const resultado = await screen.findByRole('region', { name: 'Resultado do teste' })
    expect(within(resultado).getByText('Pré-aprovado')).toBeInTheDocument()
    const limite = within(resultado).getByRole('region', { name: 'Limite' })
    // Faixa B do mock: limite fixo R$ 3.000,00 a 5,90% a.m.; parcelas conferidas com dinheiro.Parcela do Go.
    expect(within(limite).getByText('R$ 3.000,00')).toBeInTheDocument()
    expect(within(limite).getByText('5,90% a.m.')).toBeInTheDocument()
    const linhas = within(limite).getAllByRole('row').slice(1)
    expect(linhas.map((l) => l.textContent?.replace(/\s/g, ' '))).toEqual([
      '12×R$ 355,87',
      '24×R$ 236,83',
      '36×R$ 202,75',
    ])
    expect(corpo).not.toHaveProperty('pedido')
    server.events.removeAllListeners()
  })

  it('testar com só o valor barra o prazo antes de ir ao servidor', async () => {
    const usuario = userEvent.setup()
    renderizarPagina(<PoliticaPage />)
    await usuario.type(await screen.findByLabelText('CPF ou CNPJ'), '00000000434')
    await usuario.clear(screen.getByLabelText('Prazo (meses)'))
    await usuario.click(screen.getByRole('button', { name: 'Testar rascunho' }))
    expect(screen.getByLabelText('Prazo (meses)')).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByText(/ou deixe valor e prazo vazios/)).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Resultado do teste' })).not.toBeInTheDocument()
  })

  it('aba Oferta adiciona e remove prazos ofertados, e recusa prazo inválido', async () => {
    const usuario = userEvent.setup()
    renderizarPagina(<PoliticaPage />)
    await usuario.click(await screen.findByRole('tab', { name: 'Oferta' }))
    const lista = () => screen.getByRole('list', { name: 'Prazos ofertados' })
    const chips = () =>
      within(lista())
        .getAllByRole('listitem')
        .map((li) => li.textContent)
    expect(chips()).toEqual(['12 meses', '24 meses', '36 meses'])

    const novo = screen.getByLabelText('Novo prazo (meses)')
    await usuario.type(novo, '6')
    await usuario.click(screen.getByRole('button', { name: 'Adicionar' }))
    expect(chips()).toEqual(['6 meses', '12 meses', '24 meses', '36 meses'])

    await usuario.type(novo, '24')
    await usuario.click(screen.getByRole('button', { name: 'Adicionar' }))
    expect(screen.getByText('Esse prazo já está na lista.')).toBeInTheDocument()
    expect(novo).toHaveAttribute('aria-invalid', 'true')

    await usuario.clear(novo)
    await usuario.type(novo, '421')
    await usuario.click(screen.getByRole('button', { name: 'Adicionar' }))
    expect(screen.getByText('Prazo de 1 a 420 meses.')).toBeInTheDocument()

    await usuario.click(screen.getByRole('button', { name: 'Remover 24 meses' }))
    expect(chips()).toEqual(['6 meses', '12 meses', '36 meses'])
    expect(await screen.findByText('Alterações não salvas')).toBeInTheDocument()
  })

  it('prazo ofertado acima do prazo máximo é barrado ao salvar', async () => {
    const usuario = userEvent.setup()
    renderizarPagina(<PoliticaPage />)
    await usuario.click(await screen.findByRole('tab', { name: 'Oferta' }))
    const maximo = screen.getByLabelText('Prazo máximo (meses)')
    await usuario.clear(maximo)
    await usuario.type(maximo, '24')
    await usuario.click(screen.getByRole('button', { name: 'Salvar rascunho' }))
    expect(screen.getByText('Prazos acima do prazo máximo (24 meses): 36.')).toBeInTheDocument()

    await usuario.type(screen.getByLabelText('Novo prazo (meses)'), '30')
    await usuario.click(screen.getByRole('button', { name: 'Adicionar' }))
    expect(screen.getByText('Acima do prazo máximo da oferta (24 meses).')).toBeInTheDocument()
  })
})
