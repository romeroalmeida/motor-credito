import type { ReactNode } from 'react'
import { cn } from '@/shared/lib/utils'

export type Tom = 'aprovado' | 'negado' | 'contraproposta' | 'alerta' | 'neutro'

const BOLINHA: Record<Tom, string> = {
  aprovado: 'bg-sinal-aprovado',
  negado: 'bg-sinal-negado',
  contraproposta: 'bg-sinal-contraproposta',
  alerta: 'bg-sinal-alerta',
  neutro: 'bg-sinal-neutro',
}

interface EtiquetaProps {
  tom?: Tom
  children: ReactNode
  className?: string
}

/** Chip com bolinha de cor: a cor só reforça, o texto é que diz (desenho §8). */
export function Etiqueta({ tom = 'neutro', children, className }: EtiquetaProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full bg-secondary px-2.5 py-1 text-xs font-medium',
        className,
      )}
    >
      <span aria-hidden="true" className={cn('size-2 shrink-0 rounded-full', BOLINHA[tom])} />
      {children}
    </span>
  )
}
