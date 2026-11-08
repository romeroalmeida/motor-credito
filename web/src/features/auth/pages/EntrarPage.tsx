import { zodResolver } from '@hookform/resolvers/zod'
import { ArrowLeft } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { z } from 'zod'
import { AvisoErro, Campo } from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { type ErroApi, lerErroApi } from '@/shared/utils/erroApi'
import { useSessaoStore } from '@/state/useSessaoStore'
import type { EstadoVoltar } from '../components/RotaProtegida'
import { useConfirmarDoisFatores, useEntrar } from '../hooks/useSessao'

const DESTINO_PADRAO = '/visao-geral'
const MOCKS_LIGADOS = import.meta.env.DEV && import.meta.env.VITE_USAR_MOCKS === 'on'

const esquemaSenha = z.object({
  email: z.email('Digite um e-mail válido.'),
  senha: z.string().min(1, 'Digite sua senha.'),
})

const esquemaCodigo = z.object({
  codigo: z.string().regex(/^\d{6}$/, 'O código tem 6 números.'),
})

// Só caminhos internos: "//site" ou "https://..." viraria redirecionamento aberto.
function destinoSeguro(estado: unknown): string {
  const de = (estado as EstadoVoltar | null)?.de
  return de && de.startsWith('/') && !de.startsWith('//') && !de.startsWith('/entrar') ? de : DESTINO_PADRAO
}

function mensagemLogin(erro: ErroApi, passo: 'senha' | 'codigo'): string {
  if (erro.status === 429) {
    const espera = erro.esperarSegundos
      ? ` Tente de novo em ${erro.esperarSegundos} s.`
      : ' Aguarde um pouco.'
    return `Muitas tentativas seguidas.${espera}`
  }
  if (erro.status === 401) {
    return passo === 'senha'
      ? 'E-mail ou senha não conferem.'
      : 'Código não confere ou expirou. Confira o app autenticador ou volte e entre de novo.'
  }
  return erro.mensagem
}

export function EntrarPage() {
  const eu = useSessaoStore((estado) => estado.eu)
  const local = useLocation()
  const navegar = useNavigate()
  const [passo, setPasso] = useState<'senha' | 'codigo'>('senha')
  const destino = destinoSeguro(local.state)

  if (eu) return <Navigate to={destino} replace />

  return (
    <main className="grid min-h-dvh place-items-center p-4">
      <section aria-labelledby="titulo-pagina" className="w-full max-w-sm space-y-6">
        <div className="flex items-center gap-3">
          <span
            aria-hidden="true"
            className="grid size-10 place-items-center rounded-full bg-primary text-sm font-bold text-primary-foreground"
          >
            mc
          </span>
          <span className="text-sm font-medium text-muted-foreground">Motor de Crédito</span>
        </div>
        <div className="space-y-6 rounded-card bg-card p-6 shadow-card md:p-8">
          <header className="space-y-1">
            <h1 id="titulo-pagina" className="text-2xl font-semibold tracking-tight">
              Entrar
            </h1>
            <p className="text-sm text-muted-foreground">
              {passo === 'senha'
                ? 'Passo 1 de 2 · e-mail e senha'
                : 'Passo 2 de 2 · código do app autenticador'}
            </p>
          </header>
          {passo === 'senha' ? (
            <FormSenha aoAvancar={() => setPasso('codigo')} />
          ) : (
            <FormCodigo
              aoVoltar={() => setPasso('senha')}
              aoConcluir={() => navegar(destino, { replace: true })}
            />
          )}
        </div>
        {MOCKS_LIGADOS && (
          <p className="text-center text-xs text-muted-foreground">
            Modo demonstração: senha <code className="font-mono">senha-demo</code>, código{' '}
            <code className="font-mono">123456</code>. E-mail começando com analista@ ou auditor@ troca o
            papel.
          </p>
        )}
      </section>
    </main>
  )
}

function FormSenha({ aoAvancar }: { aoAvancar: () => void }) {
  const entrar = useEntrar()
  const form = useForm<z.infer<typeof esquemaSenha>>({
    resolver: zodResolver(esquemaSenha),
    defaultValues: { email: '', senha: '' },
  })
  const { errors } = form.formState

  const enviar = form.handleSubmit((dados) => entrar.mutate(dados, { onSuccess: aoAvancar }))

  return (
    <form onSubmit={enviar} noValidate className="space-y-4">
      <Campo rotulo="E-mail" erro={errors.email?.message}>
        {(props) => (
          <Input {...props} type="email" autoComplete="username" autoFocus {...form.register('email')} />
        )}
      </Campo>
      <Campo rotulo="Senha" erro={errors.senha?.message}>
        {(props) => (
          <Input {...props} type="password" autoComplete="current-password" {...form.register('senha')} />
        )}
      </Campo>
      {entrar.isError && <AvisoErro>{mensagemLogin(lerErroApi(entrar.error), 'senha')}</AvisoErro>}
      <Button type="submit" size="lg" className="w-full" disabled={entrar.isPending}>
        {entrar.isPending ? 'Conferindo…' : 'Continuar'}
      </Button>
    </form>
  )
}

function FormCodigo({ aoVoltar, aoConcluir }: { aoVoltar: () => void; aoConcluir: () => void }) {
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
      <Button type="button" variant="ghost" className="w-full" onClick={aoVoltar}>
        <ArrowLeft aria-hidden="true" />
        Voltar
      </Button>
    </form>
  )
}
