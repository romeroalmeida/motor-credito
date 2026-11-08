import { ArrowDown, ArrowUp, CircleAlert, Trash2 } from 'lucide-react'
import { type ReactNode, useId } from 'react'
import { Button } from '@/shared/components/ui/button'
import { cn } from '@/shared/lib/utils'
import type { ProblemaRegra } from '@/shared/utils/erroApi'

interface CartaoRegraProps {
  titulo: string
  posicao: number
  total: number
  problemas: ProblemaRegra[]
  aoMover: (destino: number) => void
  aoRemover: () => void
  children: ReactNode
}

/** Uma regra (corte ou faixa) com ordem, remoção e os problemas que o servidor apontou nela. */
export function CartaoRegra({
  titulo,
  posicao,
  total,
  problemas,
  aoMover,
  aoRemover,
  children,
}: CartaoRegraProps) {
  const idTitulo = useId()
  return (
    <div
      role="group"
      aria-labelledby={idTitulo}
      className={cn(
        'space-y-4 rounded-card bg-card p-4 shadow-card md:p-5',
        problemas.length > 0 && 'ring-2 ring-destructive',
      )}
    >
      <header className="flex items-center gap-2">
        <span
          aria-hidden="true"
          className="grid size-7 shrink-0 place-items-center rounded-full bg-primary text-xs font-semibold text-primary-foreground tabular-nums"
        >
          {posicao + 1}
        </span>
        <h3 id={idTitulo} className="min-w-0 flex-1 truncate font-semibold">
          {titulo}
        </h3>
        <div className="flex items-center">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={`Subir ${titulo}`}
            disabled={posicao === 0}
            onClick={() => aoMover(posicao - 1)}
          >
            <ArrowUp aria-hidden="true" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={`Descer ${titulo}`}
            disabled={posicao === total - 1}
            onClick={() => aoMover(posicao + 1)}
          >
            <ArrowDown aria-hidden="true" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={`Remover ${titulo}`}
            onClick={aoRemover}
          >
            <Trash2 aria-hidden="true" />
          </Button>
        </div>
      </header>
      {problemas.length > 0 && (
        <ul role="alert" className="space-y-1 rounded-xl bg-secondary p-3 text-sm text-destructive">
          {problemas.map(({ caminho, motivo }) => (
            <li key={`${caminho}-${motivo}`} className="flex items-start gap-2">
              <CircleAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
              <span>{motivo}</span>
            </li>
          ))}
        </ul>
      )}
      {children}
    </div>
  )
}
