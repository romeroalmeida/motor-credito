import { NavLink, Outlet } from 'react-router-dom'
import { cn } from '@/shared/lib/utils'
import { ITENS_NAVEGACAO } from './navegacao'

export function PainelLayout() {
  return (
    <div className="min-h-dvh md:flex md:gap-6 md:p-6">
      <aside className="sticky top-0 z-10 bg-background p-3 md:top-6 md:h-[calc(100dvh-3rem)] md:p-0">
        <nav
          aria-label="Menu principal"
          className="flex items-center gap-2 overflow-x-auto rounded-card bg-card p-2 shadow-card md:h-full md:w-56 md:flex-col md:items-stretch md:p-4"
        >
          <span
            aria-hidden="true"
            className="mr-2 grid size-10 shrink-0 place-items-center rounded-full bg-primary text-sm font-bold text-primary-foreground md:mb-6"
          >
            mc
          </span>
          {ITENS_NAVEGACAO.map(({ rota, rotulo, icone: Icone }) => (
            <NavLink
              key={rota}
              to={rota}
              className={({ isActive }) =>
                cn(
                  'flex shrink-0 items-center gap-3 rounded-xl px-3 py-2 text-sm font-medium transition-colors',
                  isActive
                    ? 'bg-primary text-primary-foreground'
                    : 'text-muted-foreground hover:bg-secondary',
                )
              }
            >
              <Icone aria-hidden="true" className="size-4" />
              {rotulo}
            </NavLink>
          ))}
        </nav>
      </aside>
      <main className="flex-1 p-4 md:p-0">
        <Outlet />
      </main>
    </div>
  )
}
