import { screen } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import type { Papel } from '@/shared/types'
import { euDeTeste, renderizarApp } from '@/tests/renderizar'

function comSessao(papel: Papel) {
  server.use(http.get('*/painel/auth/eu', () => HttpResponse.json(euDeTeste(papel))))
}

describe('rotas e layout', () => {
  it('admin vê todos os links do menu e o ativo marcado', async () => {
    comSessao('admin')
    renderizarApp('/consultas')
    const menu = await screen.findByRole('navigation', { name: 'Menu principal' })
    const rotulos = [
      'Visão geral',
      'Consultas',
      'Política',
      'Chaves de API',
      'Simulador de integração',
      'Equipe',
      'Auditoria',
    ]
    for (const rotulo of rotulos) expect(screen.getByRole('link', { name: rotulo })).toBeInTheDocument()
    expect(menu.querySelectorAll('a')).toHaveLength(rotulos.length)
    expect(screen.getByRole('link', { name: 'Consultas' })).toHaveAttribute('aria-current', 'page')
  })

  it.each<[Papel, string[]]>([
    ['analista', ['Visão geral', 'Consultas', 'Política']],
    ['auditor', ['Auditoria']],
  ])('%s só vê o que o papel permite', async (papel, esperados) => {
    comSessao(papel)
    renderizarApp('/')
    const menu = await screen.findByRole('navigation', { name: 'Menu principal' })
    const links = [...menu.querySelectorAll('a')].map((a) => a.textContent)
    expect(links).toEqual(esperados)
  })

  it('papel sem acesso que digita a URL vê o aviso, não a tela', async () => {
    comSessao('analista')
    renderizarApp('/chaves')
    expect(await screen.findByText('Seu papel não dá acesso a esta área')).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Chaves de API' })).not.toBeInTheDocument()
  })

  it('Entrar fica fora do layout', async () => {
    renderizarApp('/entrar')
    expect(await screen.findByRole('heading', { name: 'Entrar' })).toBeInTheDocument()
    expect(screen.queryByRole('navigation')).not.toBeInTheDocument()
  })
})
