import { zodResolver } from '@hookform/resolvers/zod'
import { ArrowLeft } from 'lucide-react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { AvisoErro, Campo } from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { lerErroApi } from '@/shared/utils/erroApi'
import { useConfirmarDoisFatores } from '../hooks/useSessao'
import { mensagemLogin } from '../utils/mensagens'

const esquemaCodigo = z.object({
  codigo: z.string().regex(/^\d{6}$/, 'O código tem 6 números.'),
})

export function FormCodigo({ aoVoltar, aoConcluir }: { aoVoltar?: () => void; aoConcluir: () => void }) {
  const confirmar = useConfirmarDoisFatores()
  const form = useForm<z.infer<typeof esquemaCodigo>>({
    resolver: zodResolver(esquemaCodigo),
    defaultValues: { codigo: '' },
  })
  const { errors } = form.formState

  const enviar = form.handleSubmit(({ codigo }) => confirmar.mutate(codigo, { onSuccess: aoConcluir }))

  return (
    <form onSubmit={enviar} noValidate className="space-y-4">
      <Campo
        rotulo="Código de 6 números"
        ajuda="Abra o app autenticador (Google Authenticator, 1Password…) e digite o código atual."
        erro={errors.codigo?.message}
      >
        {(props) => (
          <Input
            {...props}
            inputMode="numeric"
            autoComplete="one-time-code"
            maxLength={6}
            autoFocus
            className="h-12 text-center font-mono text-xl tracking-[0.4em] md:text-xl"
            {...form.register('codigo')}
          />
        )}
      </Campo>
      {confirmar.isError && <AvisoErro>{mensagemLogin(lerErroApi(confirmar.error), 'codigo')}</AvisoErro>}
      <Button type="submit" size="lg" className="w-full" disabled={confirmar.isPending}>
        {confirmar.isPending ? 'Conferindo…' : 'Entrar'}
      </Button>
      {aoVoltar && (
        <Button type="button" variant="ghost" className="w-full" onClick={aoVoltar}>
          <ArrowLeft aria-hidden="true" />
          Voltar
        </Button>
      )}
    </form>
  )
}
