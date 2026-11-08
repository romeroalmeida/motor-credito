import { CircleAlert, type LucideIcon, RotateCw } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from '@/shared/components/ui/button'
import { Skeleton } from '@/shared/components/ui/skeleton'
import { cn } from '@/shared/lib/utils'

interface CarregandoProps {
  rotulo?: string
  linhas?: number
  className?: string
}

/** Esqueleto no lugar do conteúdo; o texto fica para leitor de tela. */
export function Carregando({ rotulo = 'Carregando…', linhas = 3, className }: CarregandoProps) {
  return (
    <div role="status" aria-live="polite" className={cn('space-y-3', className)}>
      <span className="sr-only">{rotulo}</span>
      {Array.from({ length: linhas }, (_, i) => (
        <Skeleton key={i} className={cn('h-10 rounded-xl', i === linhas - 1 && 'w-2/3')} />
      ))}
    </div>
  )
}

interface VazioProps {
  icone: LucideIcon
  titulo: string
  children?: ReactNode
  acao?: ReactNode
}

export function Vazio({ icone: Icone, titulo, children, acao }: VazioProps) {
  return (
    <div className="flex flex-col items-center gap-3 px-4 py-10 text-center">
      <span
        aria-hidden="true"
        className="grid size-12 place-items-center rounded-full bg-primary text-primary-foreground"
      >
        <Icone className="size-5" />
      </span>
      <p className="font-medium">{titulo}</p>
      {children && <div className="max-w-sm text-sm text-muted-foreground">{children}</div>}
      {acao}
    </div>
  )
}

interface ErroCarregarProps {
  mensagem: string
  aoTentarDeNovo?: () => void
}

export function ErroCarregar({ mensagem, aoTentarDeNovo }: ErroCarregarProps) {
  return (
    <div role="alert" className="flex flex-col items-center gap-3 px-4 py-10 text-center">
      <CircleAlert aria-hidden="true" className="size-6 text-destructive" />
      <p className="max-w-sm text-sm">{mensagem}</p>
      {aoTentarDeNovo && (
        <Button variant="outline" onClick={aoTentarDeNovo}>
          <RotateCw aria-hidden="true" />
          Tentar de novo
        </Button>
      )}
    </div>
  )
}

/** Erro curto dentro de formulário ou ao lado de uma ação. */
export function AvisoErro({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <p role="alert" className={cn('flex items-start gap-2 text-sm text-destructive', className)}>
      <CircleAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
      <span>{children}</span>
    </p>
  )
}
