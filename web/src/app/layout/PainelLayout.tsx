import { LogOut } from 'lucide-react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useSair } from '@/features/auth'
import { Button } from '@/shared/components/ui/button'
import { cn } from '@/shared/lib/utils'
import { podeVer, ROTULO_PAPEL } from '@/shared/utils/permissoes'
import { useSessaoStore } from '@/state/useSessaoStore'
import { ITENS_NAVEGACAO } from './navegacao'

export function PainelLayout() {
  const eu = useSessaoStore((estado) => estado.eu)
  const sair = useSair()
  const navegar = useNavigate()
  const itens = ITENS_NAVEGACAO.filter((item) => podeVer(eu?.papel ?? null, item.area))

  function encerrar() {
    sair.mutate(undefined, { onSettled: () => navegar('/entrar', { replace: true }) })
  }

  return (
    <div className="min-h-dvh md:flex md:gap-6 md:p-6">
      <a
        href="#conteudo"
        className="sr-only z-50 rounded-xl bg-primary px-4 py-2 text-primary-foreground focus:not-sr-only focus:fixed focus:top-3 focus:left-3"
      >
        Pular para o conteúdo
      </a>
      <aside className="sticky top-0 z-10 bg-background p-3 md:top-6 md:h-[calc(100dvh-3rem)] md:p-0">
        <div className="flex items-center gap-2 rounded-card bg-card p-2 shadow-card md:h-full md:w-60 md:flex-col md:items-stretch md:p-4">
          <div className="flex shrink-0 items-center gap-3 md:mb-6">
            <span
              aria-hidden="true"
              className="grid size-10 place-items-center rounded-full bg-primary text-sm font-bold text-primary-foreground"
            >
              mc
            </span>
            <span className="hidden min-w-0 text-sm leading-tight md:block">
              <span className="block font-semibold">{eu?.tenant.nome}</span>
              <span className="block text-xs text-muted-foreground">Motor de Crédito</span>
            </span>
          </div>
          <nav
            aria-label="Menu principal"
            className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto md:flex-col md:items-stretch"
          >
            {itens.map(({ rota, rotulo, icone: Icone }) => (
              <NavLink
                key={rota}
                to={rota}
                className={({ isActive }) =>
                  cn(
                    'flex shrink-0 items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors',
                    isActive
                      ? 'bg-primary text-primary-foreground'
                      : 'text-muted-foreground hover:bg-secondary hover:text-foreground',
                  )
                }
              >
                <Icone aria-hidden="true" className="size-4" />
                {rotulo}
              </NavLink>
            ))}
          </nav>
          <div className="flex shrink-0 items-center gap-2 md:mt-4 md:border-t md:pt-4">
            <div className="hidden min-w-0 flex-1 text-sm leading-tight md:block">
              <p className="truncate font-medium">{eu?.usuario.nome}</p>
              <p className="text-xs text-muted-foreground">{eu && ROTULO_PAPEL[eu.papel]}</p>
            </div>
            <Button
              variant="ghost"
              size="icon"
              onClick={encerrar}
              disabled={sair.isPending}
              aria-label="Sair"
              title="Sair"
            >
              <LogOut aria-hidden="true" />
            </Button>
          </div>
        </div>
      </aside>
      <main
        id="conteudo"
        tabIndex={-1}
        className="min-w-0 flex-1 space-y-6 p-4 pb-12 outline-none md:p-0 md:pb-12"
      >
        <Outlet />
      </main>
    </div>
  )
}
