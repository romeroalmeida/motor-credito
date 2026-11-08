import { render, screen } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router-dom'
import { rotas } from '@/app/routes'

function renderizar(caminho: string) {
  render(<RouterProvider router={createMemoryRouter(rotas, { initialEntries: [caminho] })} />)
}

describe('rotas e layout', () => {
  it('mostra todos os links do menu e marca o ativo', async () => {
    renderizar('/consultas')
    const menu = await screen.findByRole('navigation', { name: 'Menu principal' })
    const rotulos = ['Visão geral', 'Consultas', 'Política', 'Chaves de API', 'Equipe']
    for (const rotulo of rotulos) expect(screen.getByRole('link', { name: rotulo })).toBeInTheDocument()
    expect(menu.querySelectorAll('a')).toHaveLength(rotulos.length)
    expect(screen.getByRole('link', { name: 'Consultas' })).toHaveAttribute('aria-current', 'page')
  })

  it('Entrar fica fora do layout', async () => {
    renderizar('/entrar')
    expect(await screen.findByRole('heading', { name: 'Entrar' })).toBeInTheDocument()
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
  })
})
