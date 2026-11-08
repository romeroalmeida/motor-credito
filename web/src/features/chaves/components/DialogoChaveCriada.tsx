import { SegredoUmaVez } from '@/shared/components'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/shared/components/ui/alert-dialog'
import type { ChaveCriada } from '../services/chavesService'

const AVISO_ID = 'Vai no cabeçalho Authorization: Bearer de cada chamada. Identifica a chave; não é segredo.'
const AVISO_SEGREDO =
  'Assina cada chamada (X-Assinatura) e nunca vai em cabeçalho. Copie e guarde num cofre de segredos agora. Não mostramos de novo: se perder, gire a chave para gerar outra.'

interface DialogoChaveCriadaProps {
  chave: ChaveCriada | null
  /** Ao fechar, quem chama descarta o segredo: ele não volta mais. */
  aoFechar: () => void
}

/** Mostra id + segredo de uma chave recém-criada. Não fecha clicando fora: o segredo some ao fechar. */
export function DialogoChaveCriada({ chave, aoFechar }: DialogoChaveCriadaProps) {
  return (
    <AlertDialog open={chave !== null} onOpenChange={(aberto) => !aberto && aoFechar()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Sua nova chave</AlertDialogTitle>
          <AlertDialogDescription>
            A integração precisa dos dois valores. O segredo aparece só agora.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <SegredoUmaVez rotulo="Id da chave" valor={chave?.chave ?? ''} aviso={AVISO_ID} />
        <SegredoUmaVez rotulo="Segredo da chave" valor={chave?.segredo ?? ''} aviso={AVISO_SEGREDO} />
        <AlertDialogFooter>
          <AlertDialogAction onClick={aoFechar}>Já guardei, fechar</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
