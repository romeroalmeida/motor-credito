import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderizarPagina } from '@/tests/renderizar'
import { EquipePage } from './EquipePage'

describe('equipe', () => {
  it('troca o papel e remove um membro depois de confirmar', async () => {
    const usuario = userEvent.setup()
    renderizarPagina(<EquipePage />)
    const papelBruno = await screen.findByRole('combobox', { name: 'Papel de Bruno Costa' })

    await usuario.selectOptions(papelBruno, 'auditor')
    await waitFor(() =>
      expect(screen.getByRole('combobox', { name: 'Papel de Bruno Costa' })).toHaveValue('auditor'),
    )

    await usuario.click(screen.getByRole('button', { name: 'Remover Carla Mendes' }))
    const dialogo = await screen.findByRole('alertdialog', { name: 'Remover Carla Mendes da equipe?' })
    expect(within(dialogo).getByText(/sessões abertas caem/)).toBeInTheDocument()
    expect(screen.getByText('Carla Mendes')).toBeInTheDocument()

    await usuario.click(within(dialogo).getByRole('button', { name: 'Remover' }))
    expect(await screen.findByRole('combobox', { name: 'Papel de Ana Ribeiro' })).toBeInTheDocument()
    await expect.poll(() => screen.queryByText('Carla Mendes')).toBeNull()
  })

  it('não remove nem rebaixa o último admin', async () => {
    const usuario = userEvent.setup()
    renderizarPagina(<EquipePage />)
    await usuario.click(await screen.findByRole('button', { name: 'Remover Ana Ribeiro' }))
    const dialogo = await screen.findByRole('alertdialog')
    await usuario.click(within(dialogo).getByRole('button', { name: 'Remover' }))
    expect(await within(dialogo).findByText('A equipe precisa de ao menos um admin.')).toBeInTheDocument()
    await usuario.click(within(dialogo).getByRole('button', { name: /Cancelar|Voltar/ }))

    await usuario.selectOptions(screen.getByRole('combobox', { name: 'Papel de Ana Ribeiro' }), 'analista')
    expect(await screen.findByText('A equipe precisa de ao menos um admin.')).toBeInTheDocument()
    expect(await screen.findByRole('combobox', { name: 'Papel de Ana Ribeiro' })).toHaveValue('admin')
  })
})
