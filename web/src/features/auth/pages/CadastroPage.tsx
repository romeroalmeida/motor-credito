import { zodResolver } from '@hookform/resolvers/zod'
import type { ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { z } from 'zod'
import { AvisoErro, Campo, Carregando } from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { type ErroApi, lerErroApi } from '@/shared/utils/erroApi'
import { FormCodigo } from '../components/FormCodigo'
import { useCadastrar, useQrCode } from '../hooks/useSessao'
import type { Cadastro } from '../services/authService'

const SENHA_MINIMA = 12

const esquema = z
  .object({
    nome: z.string().trim().min(1, 'Digite seu nome.').max(120, 'Até 120 caracteres.'),
    senha: z
      .string()
      .min(SENHA_MINIMA, `A senha precisa de ao menos ${SENHA_MINIMA} caracteres.`)
      .max(128, 'Até 128 caracteres.'),
    repetir: z.string(),
  })
  .refine((dados) => dados.senha === dados.repetir, {
    message: 'As senhas não conferem.',
    path: ['repetir'],
  })

function mensagemCadastro(erro: ErroApi): string {
  if (erro.status === 404) return 'Convite inválido, expirado ou já usado. Peça um novo a quem convidou você.'
  if (erro.status === 409) return 'Este e-mail já tem conta. Entre com ela.'
  return erro.mensagem
}

/** Aceitar convite: /cadastro?token=… → nome e senha → QR do TOTP → código → painel. */
export function CadastroPage() {
  const [parametros] = useSearchParams()
  const token = parametros.get('token') ?? ''
  const cadastrar = useCadastrar()
  const navegar = useNavigate()

  return (
    <main className="grid min-h-dvh place-items-center p-4">
      <section aria-labelledby="titulo-pagina" className="w-full max-w-sm space-y-6">
        <div className="space-y-6 rounded-card bg-card p-6 shadow-card md:p-8">
          <header className="space-y-1">
            <h1 id="titulo-pagina" className="text-2xl font-semibold tracking-tight">
              Criar sua conta
            </h1>
            <p className="text-sm text-muted-foreground">
              {cadastrar.data ? 'Passo 2 de 2 · app autenticador' : 'Passo 1 de 2 · nome e senha'}
            </p>
          </header>
          {!token ? (
            <AvisoErro>Link sem convite. Abra o link completo que você recebeu.</AvisoErro>
          ) : cadastrar.data ? (
            <PassoAutenticador cadastro={cadastrar.data}>
              <FormCodigo aoConcluir={() => navegar('/', { replace: true })} />
            </PassoAutenticador>
          ) : (
            <FormDados
              carregando={cadastrar.isPending}
              erro={cadastrar.isError ? mensagemCadastro(lerErroApi(cadastrar.error)) : undefined}
              aoEnviar={({ nome, senha }) => cadastrar.mutate({ token, nome, senha })}
            />
          )}
        </div>
        <p className="text-center text-sm text-muted-foreground">
          Já tem conta? <Link to="/entrar">Entrar</Link>
        </p>
      </section>
    </main>
  )
}

interface FormDadosProps {
  carregando: boolean
  erro?: string
  aoEnviar: (dados: z.infer<typeof esquema>) => void
}

function FormDados({ carregando, erro, aoEnviar }: FormDadosProps) {
  const form = useForm<z.infer<typeof esquema>>({
    resolver: zodResolver(esquema),
    defaultValues: { nome: '', senha: '', repetir: '' },
  })
  const { errors } = form.formState

  return (
    <form onSubmit={form.handleSubmit(aoEnviar)} noValidate className="space-y-4">
      <Campo rotulo="Nome" erro={errors.nome?.message}>
        {(props) => <Input {...props} autoComplete="name" autoFocus {...form.register('nome')} />}
      </Campo>
      <Campo rotulo="Senha" ajuda={`Ao menos ${SENHA_MINIMA} caracteres.`} erro={errors.senha?.message}>
        {(props) => (
          <Input {...props} type="password" autoComplete="new-password" {...form.register('senha')} />
        )}
      </Campo>
      <Campo rotulo="Repita a senha" erro={errors.repetir?.message}>
        {(props) => (
          <Input {...props} type="password" autoComplete="new-password" {...form.register('repetir')} />
        )}
      </Campo>
      {erro && <AvisoErro>{erro}</AvisoErro>}
      <Button type="submit" size="lg" className="w-full" disabled={carregando}>
        {carregando ? 'Criando…' : 'Continuar'}
      </Button>
    </form>
  )
}

function PassoAutenticador({ cadastro, children }: { cadastro: Cadastro; children: ReactNode }) {
  const qr = useQrCode(cadastro.otpauth_url)
  return (
    <div className="space-y-4">
      <p className="text-sm">
        Escaneie o QR code no app autenticador (Google Authenticator, 1Password…) e digite o código que
        aparecer.
      </p>
      <div className="grid place-items-center">
        {qr.data ? (
          <img
            src={qr.data}
            width={208}
            height={208}
            alt="QR code para cadastrar o motor de crédito no app autenticador"
          />
        ) : qr.isError ? (
          <AvisoErro>Não deu para desenhar o QR code. Use o código abaixo.</AvisoErro>
        ) : (
          <Carregando rotulo="Gerando QR code…" linhas={1} />
        )}
      </div>
      <p className="text-sm text-muted-foreground">
        Sem câmera? Digite este código no app:{' '}
        <code data-testid="segredo-totp" className="font-mono break-all text-foreground">
          {cadastro.segredo_totp}
        </code>
      </p>
      {children}
    </div>
  )
}
