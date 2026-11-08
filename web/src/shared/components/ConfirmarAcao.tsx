import type { ReactNode } from 'react'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/shared/components/ui/alert-dialog'
import { Button } from '@/shared/components/ui/button'

interface ConfirmarAcaoProps {
  aberto: boolean
  aoMudarAberto: (aberto: boolean) => void
  titulo: string
  descricao: ReactNode
  rotuloConfirmar: string
  aoConfirmar: () => void
  carregando?: boolean
  destrutiva?: boolean
  /** Conteúdo extra (ex.: comentário da publicação, erro da tentativa anterior). */
  children?: ReactNode
}

/**
 * Diálogo de confirmação para ação que não volta atrás (publicar, revogar, girar).
 * Não fecha sozinho ao confirmar: quem chama fecha quando a chamada der certo, e o erro aparece aqui dentro.
 */
export function ConfirmarAcao({
  aberto,
  aoMudarAberto,
  titulo,
  descricao,
  rotuloConfirmar,
  aoConfirmar,
  carregando = false,
  destrutiva = false,
  children,
}: ConfirmarAcaoProps) {
  return (
    <AlertDialog open={aberto} onOpenChange={(valor) => !carregando && aoMudarAberto(valor)}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{titulo}</AlertDialogTitle>
          <AlertDialogDescription>{descricao}</AlertDialogDescription>
        </AlertDialogHeader>
        {children}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={carregando}>Cancelar</AlertDialogCancel>
          <Button
            variant={destrutiva ? 'destructive' : 'default'}
            onClick={aoConfirmar}
            disabled={carregando}
            aria-busy={carregando}
          >
            {carregando ? 'Aguarde…' : rotuloConfirmar}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
