import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { euDeTeste, renderizarApp } from '@/tests/renderizar'
import { CODIGO_DEMO, SENHA_DEMO } from '../mocks/handlers'

const naoAutorizado = () => HttpResponse.json({ erro: 'nao_autorizado' }, { status: 401 })

describe('guarda de rota', () => {
  it('sem sessão (401 em /eu) manda para Entrar e não mostra o painel', async () => {
    server.use(http.get('*/painel/auth/eu', naoAutorizado))
    const router = renderizarApp('/consultas')
    expect(await screen.findByRole('heading', { name: 'Entrar' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/entrar')
    expect(screen.queryByRole('navigation', { name: 'Menu principal' })).not.toBeInTheDocument()
  })

  it('erro de servidor em /eu não abre o painel: mostra erro com nova tentativa', async () => {
    server.use(
      http.get('*/painel/auth/eu', () => HttpResponse.json({ erro: 'erro_interno' }, { status: 500 })),
    )
    renderizarApp('/visao-geral')
    expect(await screen.findByRole('button', { name: 'Tentar de novo' })).toBeInTheDocument()
    expect(screen.queryByRole('navigation', { name: 'Menu principal' })).not.toBeInTheDocument()
  })

  it('senha → 2FA → volta para a página que pediu', async () => {
    const usuario = userEvent.setup()
    const router = renderizarApp('/consultas')
    await screen.findByRole('heading', { name: 'Entrar' })

    await usuario.type(screen.getByLabelText('E-mail'), 'ana@credifacil.com.br')
    await usuario.type(screen.getByLabelText('Senha'), 'errada')
    await usuario.click(screen.getByRole('button', { name: 'Continuar' }))
    expect(await screen.findByText('E-mail ou senha não conferem.')).toBeInTheDocument()

    await usuario.clear(screen.getByLabelText('Senha'))
    await usuario.type(screen.getByLabelText('Senha'), SENHA_DEMO)
    await usuario.click(screen.getByRole('button', { name: 'Continuar' }))

    const codigo = await screen.findByLabelText('Código de 6 números')
    await usuario.type(codigo, CODIGO_DEMO)
    await usuario.click(screen.getByRole('button', { name: 'Entrar' }))

    expect(await screen.findByRole('heading', { name: 'Consultas' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/consultas')
  })

  it('401 em qualquer chamada depois de entrar derruba para Entrar', async () => {
    server.use(
      http.get('*/painel/auth/eu', () => HttpResponse.json(euDeTeste('admin'))),
      http.get('*/painel/visao-geral', naoAutorizado),
    )
    const router = renderizarApp('/visao-geral')
    expect(await screen.findByRole('heading', { name: 'Entrar' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/entrar')
  })
})
