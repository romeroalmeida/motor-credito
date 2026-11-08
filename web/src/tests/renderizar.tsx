import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import type { ReactElement } from 'react'
import { createMemoryRouter, MemoryRouter, RouterProvider } from 'react-router-dom'
import { rotas } from '@/app/routes'
import type { Papel, Schemas } from '@/shared/types'
import { useSessaoStore } from '@/state/useSessaoStore'

function novoClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
}

export function euDeTeste(papel: Papel): Schemas['Eu'] {
  return {
    usuario: { id: 'usr_teste', nome: 'Pessoa de Teste', email: 'teste@exemplo.com.br' },
    tenant: { id: 'ten_teste', nome: 'Tenant de Teste' },
    papel,
  }
}

export function entrarComo(papel: Papel) {
  useSessaoStore.getState().definir(euDeTeste(papel))
}

/** Uma página solta, com React Query e roteador, sem passar pela guarda. */
export function renderizarPagina(pagina: ReactElement, { papel = 'admin' as Papel | null } = {}) {
  if (papel) entrarComo(papel)
  return render(
    <QueryClientProvider client={novoClient()}>
      <MemoryRouter>{pagina}</MemoryRouter>
    </QueryClientProvider>,
  )
}

/** O app inteiro (rotas reais, guarda incluída) começando num caminho. */
export function renderizarApp(caminho: string) {
  const router = createMemoryRouter(rotas, { initialEntries: [caminho] })
  render(
    <QueryClientProvider client={novoClient()}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return router
}
