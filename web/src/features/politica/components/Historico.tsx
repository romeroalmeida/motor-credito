import { History, Undo2 } from 'lucide-react'
import { useState } from 'react'
import {
  AvisoErro,
  Campo,
  CLASSE_SELECT,
  Carregando,
  Cartao,
  ConfirmarAcao,
  ErroCarregar,
  Etiqueta,
  type Tom,
  TituloCartao,
  Vazio,
} from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { lerErroApi } from '@/shared/utils/erroApi'
import { formatarDataHora } from '@/shared/utils/formato'
import { useDiferenca, useReverter, useVersoes } from '../hooks/usePolitica'
import type { Mudanca, PoliticaVersao } from '../services/politicaService'

interface HistoricoProps {
  versaoPublicada: number | null
  podeEditar: boolean
  aoReverter: (versao: PoliticaVersao) => void
}

const TOM_MUDANCA: Record<Mudanca['tipo'], Tom> = {
  adicionado: 'aprovado',
  removido: 'negado',
  alterado: 'contraproposta',
}

function textoValor(valor: unknown): string {
  return valor === undefined ? '—' : JSON.stringify(valor)
}

export function Historico({ versaoPublicada, podeEditar, aoReverter }: HistoricoProps) {
  const versoes = useVersoes()
  const reverter = useReverter()
  const [alvo, setAlvo] = useState<number | null>(null)

  function confirmarReversao() {
    if (alvo === null) return
    reverter.mutate(alvo, {
      onSuccess: (nova) => {
        setAlvo(null)
        aoReverter(nova)
      },
    })
  }

  if (versoes.isPending) return <Carregando linhas={4} />
  if (versoes.isError) {
    return (
      <ErroCarregar mensagem={lerErroApi(versoes.error).mensagem} aoTentarDeNovo={() => versoes.refetch()} />
    )
  }
  if (versoes.data.length === 0) {
    return (
      <Cartao>
        <Vazio icone={History} titulo="Nenhuma versão publicada ainda">
          Ao publicar o rascunho, ele vira a versão 1 e passa a decidir as consultas.
        </Vazio>
      </Cartao>
    )
  }

  return (
    <div className="space-y-6">
      <Cartao aria-labelledby="titulo-versoes">
        <TituloCartao
          id="titulo-versoes"
          descricao="Versões não mudam depois de publicadas. Reverter publica uma cópia como versão nova."
        >
          Versões
        </TituloCartao>
        <ol className="divide-y divide-border">
          {versoes.data.map((versao) => (
            <li key={versao.versao} className="flex flex-wrap items-center gap-x-4 gap-y-2 py-3">
              <span className="font-semibold tabular-nums">v{versao.versao}</span>
              <div className="min-w-0 flex-1 text-sm">
                <p>
                  {versao.publicada_por} ·{' '}
                  <span className="tabular-nums">{formatarDataHora(versao.publicada_em)}</span>
                </p>
                {versao.comentario && <p className="text-muted-foreground">{versao.comentario}</p>}
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {versao.revertida_de != null && <Etiqueta>Cópia da v{versao.revertida_de}</Etiqueta>}
                {versao.versao === versaoPublicada ? (
                  <Etiqueta tom="aprovado">Em uso</Etiqueta>
                ) : (
                  podeEditar && (
                    <Button variant="outline" size="sm" onClick={() => setAlvo(versao.versao)}>
                      <Undo2 aria-hidden="true" />
                      Reverter para v{versao.versao}
                    </Button>
                  )
                )}
              </div>
            </li>
          ))}
        </ol>
      </Cartao>
      <ComparadorVersoes versoes={versoes.data.map((v) => v.versao)} versaoPublicada={versaoPublicada} />
      <ConfirmarAcao
        aberto={alvo !== null}
        aoMudarAberto={(aberto) => {
          if (!aberto) {
            setAlvo(null)
            reverter.reset()
          }
        }}
        titulo={`Reverter para a versão ${alvo}?`}
        descricao={`Uma cópia da v${alvo} vira a versão ${(versaoPublicada ?? 0) + 1} e passa a decidir as consultas novas na hora. O rascunho atual é substituído por ela.`}
        rotuloConfirmar="Reverter e publicar"
        carregando={reverter.isPending}
        aoConfirmar={confirmarReversao}
      >
        {reverter.isError && <AvisoErro>{lerErroApi(reverter.error).mensagem}</AvisoErro>}
      </ConfirmarAcao>
    </div>
  )
}

function ComparadorVersoes({
  versoes,
  versaoPublicada,
}: {
  versoes: number[]
  versaoPublicada: number | null
}) {
  const [de, setDe] = useState<number | null>(versaoPublicada ?? versoes[0] ?? null)
  const [para, setPara] = useState('rascunho')
  const diferenca = useDiferenca(de, para)

  return (
    <Cartao aria-labelledby="titulo-diferenca">
      <TituloCartao id="titulo-diferenca">Diferença</TituloCartao>
      <div className="mb-4 grid max-w-md grid-cols-2 gap-3">
        <Campo rotulo="De">
          {(props) => (
            <select
              {...props}
              className={CLASSE_SELECT}
              value={de ?? ''}
              onChange={(e) => setDe(Number(e.target.value))}
            >
              {versoes.map((v) => (
                <option key={v} value={v}>
                  v{v}
                </option>
              ))}
            </select>
          )}
        </Campo>
        <Campo rotulo="Para">
          {(props) => (
            <select
              {...props}
              className={CLASSE_SELECT}
              value={para}
              onChange={(e) => setPara(e.target.value)}
            >
              <option value="rascunho">Rascunho salvo</option>
              {versoes.map((v) => (
                <option key={v} value={String(v)}>
                  v{v}
                </option>
              ))}
            </select>
          )}
        </Campo>
      </div>
      {diferenca.isPending ? (
        <Carregando linhas={2} />
      ) : diferenca.isError ? (
        <ErroCarregar
          mensagem={lerErroApi(diferenca.error).mensagem}
          aoTentarDeNovo={() => diferenca.refetch()}
        />
      ) : diferenca.data.mudancas.length === 0 ? (
        <p className="text-sm text-muted-foreground">Sem diferença entre as duas.</p>
      ) : (
        <ul className="space-y-2">
          {diferenca.data.mudancas.map((mudanca) => (
            <li
              key={`${mudanca.caminho}-${mudanca.tipo}`}
              className="space-y-1.5 rounded-xl bg-secondary p-3"
            >
              <div className="flex flex-wrap items-center gap-2">
                <Etiqueta tom={TOM_MUDANCA[mudanca.tipo]} className="bg-card">
                  {mudanca.tipo}
                </Etiqueta>
                <code className="font-mono text-sm break-all">{mudanca.caminho}</code>
              </div>
              <p className="font-mono text-xs break-all text-muted-foreground">
                <span className="sr-only">Antes: </span>
                {textoValor(mudanca.antes)} <span aria-hidden="true">→</span>
                <span className="sr-only">, depois: </span>{' '}
                <span className="text-foreground">{textoValor(mudanca.depois)}</span>
              </p>
            </li>
          ))}
        </ul>
      )}
    </Cartao>
  )
}
