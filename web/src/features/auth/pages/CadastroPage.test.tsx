import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { euDeTeste, renderizarApp } from '@/tests/renderizar'
import { CODIGO_DEMO, TOKEN_CONVITE_DEMO } from '../mocks/handlers'

const semSessao = () => HttpResponse.json({ erro: 'nao_autorizado' }, { status: 401 })

async function preencher(usuario: ReturnType<typeof userEvent.setup>, senha: string, repetir = senha) {
  await usuario.type(screen.getByLabelText('Nome'), 'Clara Souza')
  await usuario.type(screen.getByLabelText('Senha'), senha)
  await usuario.type(screen.getByLabelText('Repita a senha'), repetir)
  await usuario.click(screen.getByRole('button', { name: 'Continuar' }))
}

describe('cadastro por convite', () => {
  beforeEach(() => server.use(http.get('*/painel/auth/eu', semSessao)))

  it('nome e senha → segredo do TOTP → código → painel', async () => {
    const usuario = userEvent.setup()
    const router = renderizarApp(`/cadastro?token=${TOKEN_CONVITE_DEMO}`)
    await screen.findByRole('heading', { name: 'Criar sua conta' })

    await preencher(usuario, 'curta')
    expect(await screen.findByText('A senha precisa de ao menos 12 caracteres.')).toBeInTheDocument()

    await usuario.clear(screen.getByLabelText('Senha'))
    await usuario.clear(screen.getByLabelText('Repita a senha'))
    await usuario.clear(screen.getByLabelText('Nome'))
    await preencher(usuario, 'uma senha bem longa')
    expect(await screen.findByTestId('segredo-totp')).toHaveTextContent('JBSWY3DPEHPK3PXP')

    // Depois do 2FA a sessão existe: a guarda vai reconfirmar em /eu.
    server.use(http.get('*/painel/auth/eu', () => HttpResponse.json(euDeTeste('admin'))))
    await usuario.type(screen.getByLabelText('Código de 6 números'), CODIGO_DEMO)
    await usuario.click(screen.getByRole('button', { name: 'Entrar' }))
    await screen.findByRole('navigation', { name: 'Menu principal' })
    await waitFor(() => expect(router.state.location.pathname).toBe('/visao-geral'))
  })

  it('convite inválido diz o que fazer', async () => {
    const usuario = userEvent.setup()
    renderizarApp('/cadastro?token=vencido')
    await screen.findByRole('heading', { name: 'Criar sua conta' })
    await preencher(usuario, 'uma senha bem longa')
    expect(await screen.findByText(/Convite inválido, expirado ou já usado/)).toBeInTheDocument()
  })
})
