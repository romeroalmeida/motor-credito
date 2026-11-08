import { Check, Copy, TriangleAlert } from 'lucide-react'
import { useState } from 'react'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/shared/components/ui/alert-dialog'
import { Button } from '@/shared/components/ui/button'

interface SegredoUmaVezProps {
  rotulo: string
  valor: string
  aviso: string
}

type EstadoCopia = 'parado' | 'copiado' | 'falhou'

/** Mostra um valor que o servidor só devolve uma vez (segredo de chave, link de convite). */
export function SegredoUmaVez({ rotulo, valor, aviso }: SegredoUmaVezProps) {
  const [copia, setCopia] = useState<EstadoCopia>('parado')

  async function copiar() {
    try {
      await navigator.clipboard.writeText(valor)
      setCopia('copiado')
    } catch {
      // Sem permissão de área de transferência: o texto fica selecionável para copiar à mão.
      setCopia('falhou')
    }
  }

  return (
    <div className="space-y-3">
      <p className="flex items-start gap-2 rounded-xl bg-secondary p-3 text-sm">
        <TriangleAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-foreground" />
        <span>{aviso}</span>
      </p>
      <div className="space-y-1.5">
        <p className="text-sm font-medium">{rotulo}</p>
        <div className="flex items-stretch gap-2">
          <code
            data-testid="segredo"
            className="min-w-0 flex-1 rounded-xl bg-primary px-3 py-2.5 font-mono text-xs break-all text-primary-foreground select-all"
          >
            {valor}
          </code>
          <Button variant="outline" onClick={copiar} className="h-auto shrink-0">
            {copia === 'copiado' ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}
            {copia === 'copiado' ? 'Copiado' : 'Copiar'}
          </Button>
        </div>
        <p aria-live="polite" className="text-xs text-muted-foreground">
          {copia === 'falhou' && 'Não deu para copiar automaticamente. Selecione o texto e copie à mão.'}
        </p>
      </div>
    </div>
  )
}

interface DialogoSegredoProps extends SegredoUmaVezProps {
  titulo: string
  aberto: boolean
  /** Ao fechar, quem chama descarta o valor: ele não volta mais. */
  aoFechar: () => void
}

/** Diálogo que não fecha clicando fora: o valor some ao fechar, então fechar é um gesto explícito. */
export function DialogoSegredo({ titulo, aberto, aoFechar, ...segredo }: DialogoSegredoProps) {
  return (
    <AlertDialog open={aberto} onOpenChange={(valor) => !valor && aoFechar()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{titulo}</AlertDialogTitle>
          <AlertDialogDescription>Este valor aparece só agora.</AlertDialogDescription>
        </AlertDialogHeader>
        <SegredoUmaVez {...segredo} />
        <AlertDialogFooter>
          <AlertDialogAction onClick={aoFechar}>Já guardei, fechar</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
