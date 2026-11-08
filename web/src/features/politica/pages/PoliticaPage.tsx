import { CircleCheck, Eye, Save, Send } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import {
  AvisoErro,
  CabecalhoPagina,
  Campo,
  Carregando,
  Cartao,
  ConfirmarAcao,
  ErroCarregar,
  Etiqueta,
} from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { Label } from '@/shared/components/ui/label'
import { Switch } from '@/shared/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/shared/components/ui/tabs'
import { Textarea } from '@/shared/components/ui/textarea'
import { lerErroApi, type ProblemaRegra } from '@/shared/utils/erroApi'
import { formatarDataHora } from '@/shared/utils/formato'
import { podeEditar } from '@/shared/utils/permissoes'
import { usePapel } from '@/state/useSessaoStore'
import { AbaContraproposta } from '../components/AbaContraproposta'
import { AbaCortes } from '../components/AbaCortes'
import { AbaFaixas } from '../components/AbaFaixas'
import { Historico } from '../components/Historico'
import { PainelTeste } from '../components/PainelTeste'
import { useEstadoPolitica, usePublicar, useSalvarRascunho } from '../hooks/usePolitica'
import type { PoliticaEstado, PoliticaVersao } from '../services/politicaService'
import { condicoesParaCel } from '../utils/campos'
import {
  type Aba,
  abaDoProblema,
  paraEditavel,
  paraPolitica,
  type PoliticaEditavel,
  problemasEm,
  validarLocal,
} from '../utils/politicaEditavel'

export function PoliticaPage() {
  const estado = useEstadoPolitica()

  if (estado.isPending) {
    return (
      <>
        <CabecalhoPagina titulo="Política" />
        <Cartao>
          <Carregando linhas={6} />
        </Cartao>
      </>
    )
  }
  if (estado.isError) {
    return (
      <>
        <CabecalhoPagina titulo="Política" />
        <Cartao>
          <ErroCarregar
            mensagem={lerErroApi(estado.error).mensagem}
            aoTentarDeNovo={() => estado.refetch()}
          />
        </Cartao>
      </>
    )
  }
  return <EditorPolitica estado={estado.data} />
}

const emJson = (p: PoliticaEditavel) => JSON.stringify(paraPolitica(p))

function EditorPolitica({ estado }: { estado: PoliticaEstado }) {
  const editavel = podeEditar(usePapel())
  const [politica, setPolitica] = useState(() => paraEditavel(estado.rascunho))
  const [salvo, setSalvo] = useState(() => emJson(politica))
  const [problemas, setProblemas] = useState<ProblemaRegra[]>([])
  const [aba, setAba] = useState<Aba>('cortes')
  const [aviso, setAviso] = useState('')
  const [publicando, setPublicando] = useState(false)
  const [comentario, setComentario] = useState('')
  const salvar = useSalvarRascunho()
  const publicar = usePublicar()

  const alterado = emJson(politica) !== salvo

  function atualizar(mudanca: Partial<PoliticaEditavel>) {
    setPolitica((atual) => ({ ...atual, ...mudanca }))
    // Os caminhos dos problemas ("faixas[1]") deixam de valer quando a lista muda.
    setProblemas([])
    setAviso('')
  }

  function mostrarProblemas(lista: ProblemaRegra[]) {
    setProblemas(lista)
    const destino = abaDoProblema(lista)
    if (destino) setAba(destino)
  }

  /** Confere o que dá para conferir aqui; devolve null e marca as regras se houver problema. */
  function politicaValida() {
    const locais = validarLocal(politica)
    if (locais.length > 0) {
      mostrarProblemas(locais)
      return null
    }
    return paraPolitica(politica)
  }

  function recusado(erro: unknown) {
    const { status, problemas: lista } = lerErroApi(erro)
    if ((status === 422 || status === 400) && lista.length > 0) mostrarProblemas(lista)
  }

  function salvarRascunho() {
    const valida = politicaValida()
    if (!valida) return
    salvar.mutate(valida, {
      onSuccess: () => {
        setSalvo(JSON.stringify(valida))
        setAviso('Rascunho salvo.')
      },
      onError: recusado,
    })
  }

  function abrirPublicacao() {
    if (!politicaValida()) return
    publicar.reset()
    setPublicando(true)
  }

  async function confirmarPublicacao() {
    const valida = politicaValida()
    if (!valida) return setPublicando(false)
    try {
      // Publicar leva o rascunho salvo: o que está na tela precisa estar salvo antes.
      if (alterado) {
        await salvar.mutateAsync(valida)
        setSalvo(JSON.stringify(valida))
      }
      const versao = await publicar.mutateAsync(comentario)
      setPublicando(false)
      setComentario('')
      setAviso(`Versão ${versao.versao} publicada. Ela já decide as consultas novas.`)
    } catch (erro) {
      const { status, problemas: lista } = lerErroApi(erro)
      if ((status === 422 || status === 400) && lista.length > 0) {
        setPublicando(false)
        mostrarProblemas(lista)
      }
    }
  }

  function aoReverter(versao: PoliticaVersao) {
    const nova = paraEditavel(versao.conteudo)
    setPolitica(nova)
    setSalvo(emJson(nova))
    setProblemas([])
    setAviso(`Versão ${versao.versao} publicada a partir da v${versao.revertida_de}.`)
  }

  function trocarModoAvancado(ligado: boolean) {
    // Ao ligar, cada regra sem CEL começa com o equivalente das condições visuais.
    const preencher = <
      T extends { cel?: string; condicoes: PoliticaEditavel['cortes'][number]['condicoes'] },
    >(
      regra: T,
    ): T => (ligado && !regra.cel?.trim() ? { ...regra, cel: condicoesParaCel(regra.condicoes) } : regra)
    atualizar({
      expressao_avancada: ligado,
      cortes: politica.cortes.map(preencher),
      faixas: politica.faixas.map(preencher),
    })
  }

  const erroPublicar = publicar.isError
    ? lerErroApi(publicar.error)
    : salvar.isError
      ? lerErroApi(salvar.error)
      : null
  const problemasSoltos = problemas.filter(({ caminho }) => !/^(cortes|faixas|contraproposta)/.test(caminho))
  const contagem = (prefixo: string) => problemasEm(problemas, prefixo).length

  return (
    <>
      <CabecalhoPagina
        titulo="Política"
        descricao={
          <>
            {estado.nome} ·{' '}
            {estado.versao_publicada
              ? `versão ${estado.versao_publicada} em uso`
              : 'nenhuma versão publicada'}
            {' · '}rascunho salvo em {formatarDataHora(estado.rascunho_atualizado_em)}
            {estado.rascunho_atualizado_por && ` por ${estado.rascunho_atualizado_por}`}
          </>
        }
        acoes={
          editavel && (
            <>
              <Button variant="outline" onClick={salvarRascunho} disabled={!alterado || salvar.isPending}>
                <Save aria-hidden="true" />
                {salvar.isPending && !publicando ? 'Salvando…' : 'Salvar rascunho'}
              </Button>
              <Button onClick={abrirPublicacao}>
                <Send aria-hidden="true" />
                Publicar
              </Button>
            </>
          )
        }
      />

      <div className="flex flex-wrap items-center gap-3" aria-live="polite">
        {alterado ? (
          <Etiqueta tom="alerta">Alterações não salvas</Etiqueta>
        ) : estado.rascunho_difere_da_publicada ? (
          <Etiqueta tom="contraproposta">Rascunho diferente da versão em uso</Etiqueta>
        ) : (
          <Etiqueta tom="aprovado">Rascunho igual à versão em uso</Etiqueta>
        )}
        {aviso && (
          <span role="status" className="inline-flex items-center gap-1.5 text-sm font-medium">
            <CircleCheck aria-hidden="true" className="size-4" />
            {aviso}
          </span>
        )}
        {!editavel && (
          <span className="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
            <Eye aria-hidden="true" className="size-4" />
            Somente leitura: só administradores editam a política.
          </span>
        )}
      </div>

      {problemas.length > 0 && (
        <div role="alert" className="space-y-2 rounded-card bg-card p-4 shadow-card ring-2 ring-destructive">
          <p className="font-medium">
            {problemas.length === 1
              ? 'Há 1 problema na política.'
              : `Há ${problemas.length} problemas na política.`}{' '}
            As regras com problema estão marcadas.
          </p>
          {problemasSoltos.map(({ caminho, motivo }) => (
            <AvisoErro key={caminho}>{motivo}</AvisoErro>
          ))}
        </div>
      )}

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_24rem]">
        <div className="min-w-0 space-y-4">
          <div className="flex items-center gap-3">
            <Switch
              id="modo-avancado"
              checked={politica.expressao_avancada}
              onCheckedChange={trocarModoAvancado}
              disabled={!editavel}
            />
            <Label htmlFor="modo-avancado">Modo avançado (expressões CEL)</Label>
          </div>
          {politica.expressao_avancada && (
            <p className="text-sm text-muted-foreground">
              As expressões CEL valem no lugar das condições visuais. O que você escrever aqui não volta para
              o editor visual.
            </p>
          )}
          <Tabs value={aba} onValueChange={(valor) => setAba(valor as Aba)}>
            <TabsList className="h-auto w-full group-data-horizontal/tabs:h-auto flex-wrap justify-start gap-1 rounded-xl bg-card p-1 shadow-card sm:w-fit">
              <AbaGatilho valor="cortes" rotulo="Corte" problemas={contagem('cortes')} />
              <AbaGatilho valor="faixas" rotulo="Faixas" problemas={contagem('faixas')} />
              <AbaGatilho valor="contraproposta" rotulo="Oferta" problemas={contagem('contraproposta')} />
              <AbaGatilho valor="historico" rotulo="Histórico" problemas={0} />
            </TabsList>
            <TabsContent value="cortes" className="pt-2">
              <Bloqueio editavel={editavel}>
                <AbaCortes
                  cortes={politica.cortes}
                  avancado={politica.expressao_avancada}
                  problemas={problemas}
                  aoMudar={(cortes) => atualizar({ cortes })}
                />
              </Bloqueio>
            </TabsContent>
            <TabsContent value="faixas" className="pt-2">
              <Bloqueio editavel={editavel}>
                <AbaFaixas
                  faixas={politica.faixas}
                  avancado={politica.expressao_avancada}
                  problemas={problemas}
                  aoMudar={(faixas) => atualizar({ faixas })}
                />
              </Bloqueio>
            </TabsContent>
            <TabsContent value="contraproposta" className="pt-2">
              <Bloqueio editavel={editavel}>
                <AbaContraproposta
                  contraproposta={politica.contraproposta}
                  problemas={problemas}
                  aoMudar={(contraproposta) => atualizar({ contraproposta })}
                />
              </Bloqueio>
            </TabsContent>
            <TabsContent value="historico" className="pt-2">
              <Historico
                versaoPublicada={estado.versao_publicada}
                podeEditar={editavel}
                aoReverter={aoReverter}
              />
            </TabsContent>
          </Tabs>
        </div>
        {editavel && (
          <aside className="min-w-0 xl:sticky xl:top-6 xl:self-start">
            <PainelTeste obterPolitica={politicaValida} aoRecusar={mostrarProblemas} />
          </aside>
        )}
      </div>

      <ConfirmarAcao
        aberto={publicando}
        aoMudarAberto={setPublicando}
        titulo={`Publicar como versão ${(estado.versao_publicada ?? 0) + 1}?`}
        descricao={
          <>
            A nova versão passa a decidir <strong>todas as consultas novas imediatamente</strong>. Versões
            publicadas não mudam; para desfazer, reverta para uma anterior no histórico.
            {alterado && ' As alterações não salvas serão salvas no rascunho antes.'}
          </>
        }
        rotuloConfirmar="Publicar agora"
        carregando={salvar.isPending || publicar.isPending}
        aoConfirmar={confirmarPublicacao}
      >
        <Campo
          rotulo="Comentário (opcional)"
          ajuda="Aparece no histórico. Ex.: taxa da faixa A de 2,90% para 2,70%."
        >
          {(props) => (
            <Textarea
              {...props}
              maxLength={500}
              value={comentario}
              onChange={(e) => setComentario(e.target.value)}
            />
          )}
        </Campo>
        {erroPublicar && erroPublicar.status !== 422 && <AvisoErro>{erroPublicar.mensagem}</AvisoErro>}
      </ConfirmarAcao>
    </>
  )
}

/** Desliga todos os controles de uma aba de uma vez para quem só pode ler. */
function Bloqueio({ editavel, children }: { editavel: boolean; children: ReactNode }) {
  return (
    <fieldset disabled={!editavel} className="min-w-0">
      <legend className="sr-only">{editavel ? 'Edição' : 'Somente leitura'}</legend>
      {children}
    </fieldset>
  )
}

function AbaGatilho({ valor, rotulo, problemas }: { valor: Aba; rotulo: string; problemas: number }) {
  return (
    <TabsTrigger
      value={valor}
      className="h-9 flex-none rounded-lg px-3 data-active:bg-primary data-active:text-primary-foreground data-active:shadow-none data-active:hover:text-primary-foreground"
    >
      {rotulo}
      {problemas > 0 && (
        <span className="ml-1 rounded-full bg-destructive px-1.5 text-xs text-white tabular-nums">
          {problemas}
          <span className="sr-only"> {problemas === 1 ? 'problema' : 'problemas'}</span>
        </span>
      )}
    </TabsTrigger>
  )
}
