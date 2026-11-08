import { zodResolver } from '@hookform/resolvers/zod'
import { MailPlus, UserMinus, Users } from 'lucide-react'
import { useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { z } from 'zod'
import {
  AvisoErro,
  CabecalhoPagina,
  Campo,
  CLASSE_SELECT,
  Carregando,
  Cartao,
  ConfirmarAcao,
  DialogoSegredo,
  ErroCarregar,
  Etiqueta,
  TituloCartao,
  Vazio,
} from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import type { Papel } from '@/shared/types'
import { lerErroApi } from '@/shared/utils/erroApi'
import { formatarData } from '@/shared/utils/formato'
import { ROTULO_PAPEL } from '@/shared/utils/permissoes'
import { useConvidar, useEquipe, useRemoverMembro, useTrocarPapel } from '../hooks/useEquipe'
import type { Membro } from '../services/equipeService'

const PAPEIS: { valor: Papel; descricao: string }[] = [
  { valor: 'analista', descricao: 'vê consultas e a política' },
  { valor: 'admin', descricao: 'edita política, chaves e equipe' },
  { valor: 'auditor', descricao: 'vê só os números gerais' },
]

const esquema = z.object({
  email: z.email('Digite um e-mail válido.'),
  papel: z.enum(['admin', 'analista', 'auditor']),
})

type Formulario = z.infer<typeof esquema>

function mensagemEquipe(erro: unknown): string {
  const lido = lerErroApi(erro)
  return lido.status === 409 ? 'A equipe precisa de ao menos um admin.' : lido.mensagem
}

export function EquipePage() {
  const equipe = useEquipe()
  const trocar = useTrocarPapel()
  const remover = useRemoverMembro()
  const [aRemover, setARemover] = useState<Membro | null>(null)

  return (
    <>
      <CabecalhoPagina titulo="Equipe" descricao="Quem acessa o painel da sua empresa e com qual papel." />
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
        <Cartao aria-labelledby="titulo-membros">
          <TituloCartao id="titulo-membros">Membros</TituloCartao>
          {equipe.isPending ? (
            <Carregando linhas={3} />
          ) : equipe.isError ? (
            <ErroCarregar
              mensagem={lerErroApi(equipe.error).mensagem}
              aoTentarDeNovo={() => equipe.refetch()}
            />
          ) : (
            <>
              {equipe.data.membros.length === 0 ? (
                <Vazio icone={Users} titulo="Ninguém na equipe ainda" />
              ) : (
                <ul className="divide-y divide-border">
                  {equipe.data.membros.map((membro) => (
                    <li key={membro.usuario_id} className="flex flex-wrap items-center gap-x-4 gap-y-1 py-3">
                      <span
                        aria-hidden="true"
                        className="grid size-9 shrink-0 place-items-center rounded-full bg-secondary text-sm font-semibold"
                      >
                        {membro.nome.charAt(0)}
                      </span>
                      <div className="min-w-0 flex-1">
                        <p className="truncate font-medium">{membro.nome}</p>
                        <p className="truncate text-sm text-muted-foreground">{membro.email}</p>
                      </div>
                      <div className="flex items-center gap-2 text-sm">
                        <div className="text-right">
                          <select
                            aria-label={`Papel de ${membro.nome}`}
                            className={CLASSE_SELECT}
                            value={membro.papel}
                            disabled={trocar.isPending}
                            onChange={(e) =>
                              trocar.mutate({ usuarioId: membro.usuario_id, papel: e.target.value as Papel })
                            }
                          >
                            {PAPEIS.map(({ valor }) => (
                              <option key={valor} value={valor}>
                                {ROTULO_PAPEL[valor]}
                              </option>
                            ))}
                          </select>
                          <p className="mt-1 text-xs text-muted-foreground tabular-nums">
                            desde {formatarData(membro.desde)}
                          </p>
                        </div>
                        <Button
                          variant="outline"
                          size="sm"
                          aria-label={`Remover ${membro.nome}`}
                          onClick={() => setARemover(membro)}
                        >
                          <UserMinus aria-hidden="true" />
                          Remover
                        </Button>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
              {trocar.isError && <AvisoErro className="mt-3">{mensagemEquipe(trocar.error)}</AvisoErro>}
              {equipe.data.convites.length > 0 && (
                <div className="mt-6 space-y-2">
                  <h3 className="text-sm font-semibold">Convites pendentes</h3>
                  <ul className="divide-y divide-border">
                    {equipe.data.convites.map((convite) => (
                      <li key={convite.id} className="flex flex-wrap items-center gap-x-4 gap-y-1 py-3">
                        <span className="min-w-0 flex-1 truncate text-sm">{convite.email}</span>
                        <Etiqueta tom="alerta">{ROTULO_PAPEL[convite.papel]} · aguardando cadastro</Etiqueta>
                        <span className="text-xs text-muted-foreground tabular-nums">
                          expira em {formatarData(convite.expira_em)}
                        </span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </>
          )}
        </Cartao>
        <FormConvite />
      </div>
      <ConfirmarAcao
        aberto={aRemover !== null}
        aoMudarAberto={(aberto) => {
          if (!aberto) {
            setARemover(null)
            remover.reset()
          }
        }}
        titulo={`Remover ${aRemover?.nome ?? ''} da equipe?`}
        descricao="A pessoa perde o acesso na hora: as sessões abertas caem. A conta continua existindo e pode ser convidada de novo."
        rotuloConfirmar="Remover"
        destrutiva
        carregando={remover.isPending}
        aoConfirmar={() =>
          aRemover && remover.mutate(aRemover.usuario_id, { onSuccess: () => setARemover(null) })
        }
      >
        {remover.isError && <AvisoErro>{mensagemEquipe(remover.error)}</AvisoErro>}
      </ConfirmarAcao>
    </>
  )
}

function FormConvite() {
  const convidar = useConvidar()
  const [link, setLink] = useState<string | null>(null)
  const form = useForm<Formulario>({
    resolver: zodResolver(esquema),
    defaultValues: { email: '', papel: 'analista' },
  })
  const { errors } = form.formState
  const papel = useWatch({ control: form.control, name: 'papel' })

  const enviar = form.handleSubmit((dados) =>
    convidar.mutate(dados, {
      onSuccess: (convite) => {
        form.reset()
        setLink(convite.link)
      },
    }),
  )

  const erro = convidar.isError ? lerErroApi(convidar.error) : null

  return (
    <Cartao aria-labelledby="titulo-convidar" className="lg:self-start">
      <TituloCartao
        id="titulo-convidar"
        descricao="A pessoa recebe um link de cadastro com senha e app autenticador."
      >
        Convidar
      </TituloCartao>
      <form onSubmit={enviar} noValidate className="space-y-4">
        <Campo rotulo="E-mail" erro={errors.email?.message}>
          {(props) => <Input {...props} type="email" autoComplete="off" {...form.register('email')} />}
        </Campo>
        <Campo rotulo="Papel" ajuda={PAPEIS.find((p) => p.valor === papel)?.descricao}>
          {(props) => (
            <select {...props} className={CLASSE_SELECT} {...form.register('papel')}>
              {PAPEIS.map(({ valor }) => (
                <option key={valor} value={valor}>
                  {ROTULO_PAPEL[valor]}
                </option>
              ))}
            </select>
          )}
        </Campo>
        {erro && (
          <AvisoErro>
            {erro.status === 409 ? 'Este e-mail já é da equipe ou já tem convite pendente.' : erro.mensagem}
          </AvisoErro>
        )}
        <Button type="submit" className="w-full" disabled={convidar.isPending}>
          <MailPlus aria-hidden="true" />
          {convidar.isPending ? 'Convidando…' : 'Enviar convite'}
        </Button>
      </form>
      <DialogoSegredo
        aberto={link !== null}
        aoFechar={() => {
          setLink(null)
          convidar.reset()
        }}
        titulo="Convite criado"
        rotulo="Link de cadastro (vale 7 dias)"
        valor={link ?? ''}
        aviso="Mande este link só para a pessoa convidada. Quem tiver o link consegue se cadastrar na sua equipe."
      />
    </Cartao>
  )
}
