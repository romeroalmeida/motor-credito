import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { euDeTeste, renderizarApp, renderizarPagina } from '@/tests/renderizar'
import { SEQ_ADULTERADO } from '../mocks/handlers'
import { AuditoriaPage } from './AuditoriaPage'

describe('auditoria', () => {
  it('lista os registros, aponta o hash que não confere e verifica a cadeia sob pedido', async () => {
    const usuario = userEvent.setup()
    renderizarPagina(<AuditoriaPage />, { papel: 'auditor' })
    expect(await screen.findByText('#60')).toBeInTheDocument()
    expect(screen.queryByText(`#${SEQ_ADULTERADO}`)).not.toBeInTheDocument()

    await usuario.click(screen.getByRole('button', { name: 'Carregar mais' }))
    const adulterado = (await screen.findByText(`#${SEQ_ADULTERADO}`)).closest('li') as HTMLElement
    expect(within(adulterado).getByText('Hash não confere')).toBeInTheDocument()

    await usuario.click(screen.getByRole('button', { name: 'Verificar integridade' }))
    const resultado = await screen.findByRole('status')
    expect(within(resultado).getByText('Cadeia quebrada')).toBeInTheDocument()
    expect(within(resultado).getByText(/seq 7 alterado/)).toBeInTheDocument()
  })

  it('auditor entra direto na auditoria e não vê consultas no menu', async () => {
    server.use(http.get('*/painel/auth/eu', () => HttpResponse.json(euDeTeste('auditor'))))
    renderizarApp('/')
    expect(await screen.findByRole('heading', { name: 'Auditoria', level: 1 })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Consultas' })).not.toBeInTheDocument()
  })
})
