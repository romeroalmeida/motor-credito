import type { ComponentProps, ReactNode } from 'react'
import { cn } from '@/shared/lib/utils'

/** Card branco de raio grande e sombra difusa (desenho §8). Elevação só pela sombra, sem borda. */
export function Cartao({ className, ...props }: ComponentProps<'section'>) {
  return (
    <section className={cn('min-w-0 rounded-card bg-card p-5 shadow-card md:p-6', className)} {...props} />
  )
}

interface TituloCartaoProps {
  id?: string
  children: ReactNode
  acoes?: ReactNode
  descricao?: ReactNode
}

export function TituloCartao({ id, children, acoes, descricao }: TituloCartaoProps) {
  return (
    <header className="mb-4 flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0 space-y-1">
        <h2 id={id} className="text-base font-semibold tracking-tight">
          {children}
        </h2>
        {descricao && <p className="text-sm text-muted-foreground">{descricao}</p>}
      </div>
      {acoes && <div className="flex flex-wrap items-center gap-2">{acoes}</div>}
    </header>
  )
}
