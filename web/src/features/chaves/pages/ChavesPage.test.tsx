import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderizarPagina } from '@/tests/renderizar'
import { ChavesPage } from './ChavesPage'

describe('chaves de API', () => {
  it('o segredo aparece uma vez só: some ao fechar e não volta na lista', async () => {
    const usuario = userEvent.setup()
    renderizarPagina(<ChavesPage />)
    await screen.findByText('produção')

    await usuario.type(screen.getByLabelText('Nome da nova chave'), 'integração nova')
    await usuario.click(screen.getByRole('button', { name: 'Criar chave' }))

    const dialogo = await screen.findByRole('alertdialog', { name: 'Sua nova chave' })
    const segredo = within(dialogo).getByTestId('segredo').textContent ?? ''
    expect(segredo).toMatch(/^ck_live_/)
    expect(within(dialogo).getByText(/Não mostramos de novo/)).toBeInTheDocument()

    await usuario.click(within(dialogo).getByRole('button', { name: 'Copiar' }))
    expect(await navigator.clipboard.readText()).toBe(segredo)
    expect(within(dialogo).getByRole('button', { name: 'Copiado' })).toBeInTheDocument()

    await usuario.click(within(dialogo).getByRole('button', { name: 'Já guardei, fechar' }))
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(await screen.findByText('integração nova')).toBeInTheDocument()
    expect(screen.queryByText(segredo)).not.toBeInTheDocument()
    expect(document.body.textContent).not.toContain(segredo)
  })

  it('revogar pede confirmação antes de agir', async () => {
    const usuario = userEvent.setup()
    renderizarPagina(<ChavesPage />)
    const item = (await screen.findByText('produção')).closest('li') as HTMLElement

    await usuario.click(within(item).getByRole('button', { name: 'Revogar' }))
    const dialogo = await screen.findByRole('alertdialog', { name: 'Revogar a chave "produção"?' })
    expect(within(item).getByText('Ativa')).toBeInTheDocument()

    await usuario.click(within(dialogo).getByRole('button', { name: 'Revogar chave' }))
    expect(await within(item).findByText('Revogada')).toBeInTheDocument()
    expect(within(item).queryByRole('button', { name: 'Revogar' })).not.toBeInTheDocument()
  })

  it('girar mostra o novo segredo uma vez', async () => {
    const usuario = userEvent.setup()
    renderizarPagina(<ChavesPage />)
    const ativa = (await screen.findAllByText('Ativa'))[0]?.closest('li') as HTMLElement
    await usuario.click(within(ativa).getByRole('button', { name: 'Girar' }))
    await usuario.click(
      within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Girar chave' }),
    )
    const dialogo = await screen.findByRole('alertdialog', { name: 'Sua nova chave' })
    expect(within(dialogo).getByTestId('segredo')).toHaveTextContent(/^ck_live_/)
  })
})
