import { type ReactNode, useId } from 'react'
import { Label } from '@/shared/components/ui/label'
import { cn } from '@/shared/lib/utils'

export interface PropsDoControle {
  id: string
  'aria-invalid': boolean
  'aria-describedby': string | undefined
}

interface CampoProps {
  rotulo: ReactNode
  ajuda?: ReactNode
  erro?: string
  className?: string
  /** Rótulo só para leitor de tela (em linhas compactas, como a condição do editor). */
  rotuloOculto?: boolean
  children: (props: PropsDoControle) => ReactNode
}

/** Rótulo + controle + ajuda + erro, com os ids ligados para leitor de tela. */
export function Campo({ rotulo, ajuda, erro, className, rotuloOculto, children }: CampoProps) {
  const id = useId()
  const idAjuda = ajuda ? `${id}-ajuda` : undefined
  const idErro = erro ? `${id}-erro` : undefined
  const descritores = [idErro, idAjuda].filter(Boolean).join(' ') || undefined
  return (
    <div className={cn('flex min-w-0 flex-col gap-1.5', className)}>
      <Label htmlFor={id} className={cn(rotuloOculto && 'sr-only')}>
        {rotulo}
      </Label>
      {children({ id, 'aria-invalid': Boolean(erro), 'aria-describedby': descritores })}
      {erro && (
        <p id={idErro} className="text-xs font-medium text-destructive">
          {erro}
        </p>
      )}
      {ajuda && (
        <p id={idAjuda} className="text-xs text-muted-foreground">
          {ajuda}
        </p>
      )}
    </div>
  )
}

/** Select nativo com o mesmo visual do Input: no celular abre o seletor do sistema. */
export const CLASSE_SELECT =
  'h-10 w-full min-w-0 rounded-xl border border-borda-campo bg-card px-3 text-base md:text-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive'
