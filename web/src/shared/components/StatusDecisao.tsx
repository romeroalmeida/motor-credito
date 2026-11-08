import { cn } from '@/shared/lib/utils'
import type { StatusDecisao as Status } from '@/shared/types'

const APARENCIA: Record<Status, { rotulo: string; bolinha: string }> = {
  aprovado: { rotulo: 'Aprovado', bolinha: 'bg-sinal-aprovado' },
  negado: { rotulo: 'Negado', bolinha: 'bg-sinal-negado' },
  contraproposta: { rotulo: 'Contraproposta', bolinha: 'bg-sinal-contraproposta' },
  documento_invalido: { rotulo: 'Documento inválido', bolinha: 'bg-sinal-neutro' },
  em_analise: { rotulo: 'Em análise', bolinha: 'bg-sinal-alerta' },
}

interface StatusDecisaoProps {
  status: Status
  className?: string
}

// Cor nunca sozinha: a bolinha é decorativa, o texto carrega o significado (desenho §8).
export function StatusDecisao({ status, className }: StatusDecisaoProps) {
  const { rotulo, bolinha } = APARENCIA[status]
  return (
    <span className={cn('inline-flex items-center gap-2 text-sm font-medium', className)}>
      <span aria-hidden="true" data-testid="bolinha" className={cn('size-2.5 rounded-full', bolinha)} />
      {rotulo}
    </span>
  )
}
