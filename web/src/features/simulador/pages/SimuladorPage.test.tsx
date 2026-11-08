import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { renderizarPagina } from '@/tests/renderizar'
import { SimuladorPage } from './SimuladorPage'

async function abrir() {
  const usuario = userEvent.setup()
  renderizarPagina(<SimuladorPage />)
  await screen.findByLabelText('Chave de API')
  return usuario
}

const exemplo = (nome: string) =>
  within(screen.getByRole('group', { name: 'Exemplos' })).getByRole('button', { name: nome })

describe('simulador de integração', () => {
  it('exemplos preenchem o formulário', async () => {
    const usuario = await abrir()
    await usuario.click(exemplo('Contraproposta'))
    expect(screen.getByLabelText('CPF ou CNPJ')).toHaveValue('000.000.004-34')
    expect(screen.getByLabelText('Valor pedido (R$)')).toHaveValue('50.000,00')

    await usuario.click(exemplo('Serpro'))
    expect(screen.getByLabelText('CPF ou CNPJ')).toHaveValue('404.428.201-35')
    expect(screen.getByLabelText('Nascimento')).toHaveValue('1970-11-14')
    expect(screen.getByText(/cota do trial/)).toBeInTheDocument()

    await usuario.click(exemplo('CNPJ real'))
    expect(screen.getByLabelText('CPF ou CNPJ')).toHaveValue('00.000.000/0001-91')
    expect(screen.getByLabelText('Nascimento')).toHaveValue('')
  })

  it('envia, mostra requisição, resposta e código, e nunca o segredo', async () => {
    let enviado: unknown
    server.events.on('request:start', async ({ request }) => {
      if (request.url.endsWith('/painel/simulador/consultas')) enviado = await request.clone().json()
    })
    const usuario = await abrir()
    await usuario.click(exemplo('Aprovado'))
    await usuario.click(screen.getByRole('button', { name: 'Enviar consulta' }))

    const resposta = await screen.findByRole('region', { name: 'Resposta' })
    expect(within(resposta).getByText('Aprovado')).toBeInTheDocument()
    expect(within(resposta).getByText('200')).toBeInTheDocument()
    expect(within(resposta).getByRole('link', { name: /Ver no painel/ })).toHaveAttribute(
      'href',
      expect.stringMatching(/^\/consultas\/cons_/),
    )
    const requisicao = screen.getByRole('region', { name: 'Requisição' })
    expect(within(requisicao).getByRole('rowheader', { name: 'X-Assinatura' })).toBeInTheDocument()
    expect(within(requisicao).getByText(/Bearer ck_live_/)).toBeInTheDocument()

    expect(enviado).toMatchObject({
      chave_id: expect.any(String),
      requisicao: { documento: '000.000.004-34' },
    })
    expect(JSON.stringify(enviado)).not.toMatch(/segredo/i)
    // Único hex de 64 na tela é a X-Assinatura; nenhum segredo de chave.
    const textos: string[] = []
    const andador = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
    while (andador.nextNode()) textos.push(andador.currentNode.textContent ?? '')
    const hex = textos.flatMap((t) => t.match(/\b[0-9a-f]{64}\b/g) ?? [])
    expect(new Set(hex)).toEqual(new Set(['f'.repeat(64)]))
    expect(screen.getByText(/O código calcula a assinatura/)).toHaveTextContent(
      'Defina MOTOR_SEGREDO com o segredo',
    )
    server.events.removeAllListeners()
  })

  it('consulta sem valor e prazo mostra Pré-aprovado com o limite e não manda pedido', async () => {
    let enviado: { requisicao?: Record<string, unknown> } | undefined
    server.events.on('request:start', async ({ request }) => {
      if (request.url.endsWith('/painel/simulador/consultas')) enviado = await request.clone().json()
    })
    const usuario = await abrir()
    await usuario.click(exemplo('Pré-aprovação'))
    expect(screen.getByLabelText('Valor pedido (R$)')).toHaveValue('')
    expect(screen.getByLabelText('Prazo (meses)')).toHaveValue('')
    await usuario.click(screen.getByRole('button', { name: 'Enviar consulta' }))

    const resposta = await screen.findByRole('region', { name: 'Resposta' })
    expect(within(resposta).getByText('Pré-aprovado')).toBeInTheDocument()
    const limite = within(resposta).getByRole('region', { name: 'Limite' })
    expect(within(limite).getByText('R$ 30.000,00')).toBeInTheDocument()
    expect(within(limite).getByText('2,90% a.m.')).toBeInTheDocument()
    expect(within(limite).getAllByRole('row')).toHaveLength(4)
    expect(enviado?.requisicao).not.toHaveProperty('pedido')
    expect(enviado?.requisicao).toMatchObject({ dados: { renda: 500000 } })
    server.events.removeAllListeners()
  })

  it('só o valor, sem prazo, é barrado antes de enviar', async () => {
    const usuario = await abrir()
    await usuario.click(exemplo('Aprovado'))
    await usuario.clear(screen.getByLabelText('Prazo (meses)'))
    await usuario.click(screen.getByRole('button', { name: 'Enviar consulta' }))
    expect(screen.getByLabelText('Prazo (meses)')).toHaveAttribute('aria-invalid', 'true')
    expect(screen.queryByRole('region', { name: 'Resposta' })).not.toBeInTheDocument()
  })

  it('abas trocam o código entre curl, Node e Python', async () => {
    const usuario = await abrir()
    await usuario.click(exemplo('Aprovado'))
    await usuario.click(screen.getByRole('button', { name: 'Enviar consulta' }))
    const codigo = await screen.findByRole('region', { name: 'Código' })
    expect(within(codigo).getByLabelText('Código curl')).toHaveTextContent('curl -sS')

    await usuario.click(within(codigo).getByRole('tab', { name: 'Node' }))
    expect(within(codigo).getByLabelText('Código Node')).toHaveTextContent('process.env.MOTOR_SEGREDO')
    await usuario.click(within(codigo).getByRole('tab', { name: 'Python' }))
    expect(within(codigo).getByLabelText('Código Python')).toHaveTextContent('os.environ["MOTOR_SEGREDO"]')

    await usuario.click(within(codigo).getByRole('button', { name: 'Copiar código Python' }))
    expect(await navigator.clipboard.readText()).toContain('MOTOR_SEGREDO')
  })

  it('reusar a última Idempotency-Key devolve a mesma consulta', async () => {
    const usuario = await abrir()
    expect(screen.getByRole('button', { name: 'Reusar a última' })).toBeDisabled()
    await usuario.click(exemplo('Aprovado'))
    await usuario.click(screen.getByRole('button', { name: 'Enviar consulta' }))
    const link = await screen.findByRole('link', { name: /Ver no painel/ })
    const primeira = link.getAttribute('href')
    const idem = within(screen.getByRole('region', { name: 'Requisição' })).getByRole('rowheader', {
      name: 'Idempotency-Key',
    }).nextElementSibling?.textContent

    await usuario.click(screen.getByRole('button', { name: 'Reusar a última' }))
    expect(screen.getByLabelText('Idempotency-Key')).toHaveValue(idem)
    await usuario.click(screen.getByRole('button', { name: 'Enviar consulta' }))
    expect(await screen.findByRole('link', { name: /Ver no painel/ })).toHaveAttribute('href', primeira)

    await usuario.click(screen.getByRole('button', { name: 'Nova' }))
    expect(screen.getByLabelText('Idempotency-Key')).not.toHaveValue(idem)
  })

  it('401 da API vem explicado', async () => {
    server.use(
      http.post('*/painel/simulador/consultas', () =>
        HttpResponse.json({
          requisicao: { metodo: 'POST', url: 'http://x/v1/consultas', cabecalhos: {}, corpo: '{}' },
          resposta: { status: 401, cabecalhos: {}, corpo: { erro: 'nao_autorizado' }, duracao_ms: 3 },
          consulta_id: null,
          trechos: { curl: '', node: '', python: '' },
        }),
      ),
    )
    const usuario = await abrir()
    await usuario.click(exemplo('Aprovado'))
    await usuario.click(screen.getByRole('button', { name: 'Enviar consulta' }))
    expect(await screen.findByText(/não está na/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'lista de IPs permitidos' })).toHaveAttribute('href', '/chaves')
    expect(screen.queryByRole('link', { name: /Ver no painel/ })).not.toBeInTheDocument()
  })

  it('sem chave ativa, leva para criar uma', async () => {
    server.use(http.get('*/painel/chaves', () => HttpResponse.json({ itens: [] })))
    renderizarPagina(<SimuladorPage />)
    expect(await screen.findByText('Nenhuma chave de API ativa')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Criar uma chave' })).toHaveAttribute('href', '/chaves')
  })
})
