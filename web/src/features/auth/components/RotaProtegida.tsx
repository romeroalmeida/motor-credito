import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { Carregando, ErroCarregar } from '@/shared/components'
import { lerErroApi, statusDoErro } from '@/shared/utils/erroApi'
import { useSessaoStore } from '@/state/useSessaoStore'
import { useEu } from '../hooks/useSessao'

export interface EstadoVoltar {
  de?: string
}

/**
 * Guarda das rotas do painel. Fail-closed: só mostra o painel com uma sessão confirmada por
 * GET /painel/auth/eu. 401 (aqui ou em qualquer chamada, via apiClient) leva para Entrar.
 */
export function RotaProtegida() {
  const eu = useSessaoStore((estado) => estado.eu)
  const sessao = useEu()
  const local = useLocation()

  if (eu) return <Outlet />

  if (sessao.isPending) {
    return (
      <div className="mx-auto max-w-md p-6 pt-24">
        <Carregando rotulo="Verificando sua sessão…" />
      </div>
    )
  }

  if (sessao.isError && statusDoErro(sessao.error) !== 401) {
    return (
      <div className="mx-auto max-w-md p-6 pt-24">
        <ErroCarregar mensagem={lerErroApi(sessao.error).mensagem} aoTentarDeNovo={() => sessao.refetch()} />
      </div>
    )
  }

  const voltar: EstadoVoltar = { de: `${local.pathname}${local.search}` }
  return <Navigate to="/entrar" replace state={voltar} />
}
