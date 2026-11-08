import { Inbox } from 'lucide-react'
import { useState } from 'react'
import {
  CabecalhoPagina,
  Carregando,
  Cartao,
  ErroCarregar,
  Etiqueta,
  StatusDecisao,
  TituloCartao,
  Vazio,
} from '@/shared/components'
import { cn } from '@/shared/lib/utils'
import type { StatusDecisao as Status } from '@/shared/types'
import { lerErroApi } from '@/shared/utils/erroApi'
import { formatarInteiro, formatarPercentualBps } from '@/shared/utils/formato'
import { rotuloSinal } from '@/shared/utils/rotulos'
import { useVisaoGeral } from '../hooks/useVisaoGeral'
import type { Periodo, VisaoGeral } from '../services/visaoGeralService'

const PERIODOS: { valor: Periodo; rotulo: string }[] = [
  { valor: 'hoje', rotulo: 'Hoje' },
  { valor: '7d', rotulo: '7 dias' },
  { valor: '30d', rotulo: '30 dias' },
]

const ORDEM_STATUS: Status[] = [
  'aprovado',
  'contraproposta',
  'pre_aprovado',
  'negado',
  'documento_invalido',
  'em_analise',
]

const COR_BARRA: Record<Status, string> = {
  aprovado: 'bg-sinal-aprovado',
  contraproposta: 'bg-sinal-contraproposta',
  pre_aprovado: 'bg-sinal-pre-aprovado',
  negado: 'bg-sinal-negado',
  documento_invalido: 'bg-sinal-neutro',
  em_analise: 'bg-sinal-alerta',
}

/** Parte de um total em pontos-base, sem float no resultado. */
function proporcaoBps(parte: number, total: number): number {
  return total === 0 ? 0 : Math.round((parte * 10000) / total)
}

export function VisaoGeralPage() {
  const [periodo, setPeriodo] = useState<Periodo>('hoje')
  const visao = useVisaoGeral(periodo)

  return (
    <>
      <CabecalhoPagina
        titulo="Visão geral"
        descricao="Como o motor está decidindo no período."
        acoes={
          <div role="group" aria-label="Período" className="flex gap-1 rounded-xl bg-card p-1 shadow-card">
            {PERIODOS.map(({ valor, rotulo }) => (
              <button
                key={valor}
                type="button"
                aria-pressed={periodo === valor}
                onClick={() => setPeriodo(valor)}
                className={cn(
                  'rounded-lg px-3 py-1.5 text-sm font-medium transition-colors',
                  periodo === valor
                    ? 'bg-primary text-primary-foreground'
                    : 'text-muted-foreground hover:bg-secondary',
                )}
              >
                {rotulo}
              </button>
            ))}
          </div>
        }
      />
      {visao.isPending ? (
        <Cartao>
          <Carregando linhas={4} />
        </Cartao>
      ) : visao.isError ? (
        <Cartao>
          <ErroCarregar mensagem={lerErroApi(visao.error).mensagem} aoTentarDeNovo={() => visao.refetch()} />
        </Cartao>
      ) : visao.data.total === 0 ? (
        <Cartao>
          <Vazio icone={Inbox} titulo="Nenhuma consulta neste período">
            Quando sua integração chamar <code className="font-mono">POST /v1/consultas</code>, os números
            aparecem aqui.
          </Vazio>
        </Cartao>
      ) : (
        <Painel visao={visao.data} atualizando={visao.isFetching} />
      )}
    </>
  )
}

function Painel({ visao, atualizando }: { visao: VisaoGeral; atualizando: boolean }) {
  const maiorFaixa = Math.max(...visao.distribuicao_score.map((f) => f.quantidade), 1)
  const maiorSinal = Math.max(...visao.sinais_frequentes.map((s) => s.quantidade), 1)
  const status = ORDEM_STATUS.filter((s) => (visao.por_status[s] ?? 0) > 0)

  return (
    <div aria-busy={atualizando} className={cn('grid gap-6 lg:grid-cols-3', atualizando && 'opacity-70')}>
      <Cartao aria-labelledby="titulo-decisoes" className="lg:col-span-2">
        <TituloCartao id="titulo-decisoes">Decisões</TituloCartao>
        <div className="flex flex-wrap items-end gap-x-10 gap-y-4">
          <p>
            <span className="block text-4xl font-semibold tracking-tight tabular-nums">
              {formatarInteiro(visao.total)}
            </span>
            <span className="text-sm text-muted-foreground">consultas</span>
          </p>
          <p>
            <span className="block text-4xl font-semibold tracking-tight tabular-nums">
              {formatarPercentualBps(visao.taxa_aprovacao_bps)}
            </span>
            <span className="text-sm text-muted-foreground">
              aprovadas ou com contraproposta, entre as com pedido
            </span>
          </p>
          {/* Pré-aprovação não soma em aprovação: cada taxa sobre o próprio tipo de consulta. */}
          {(visao.por_status.pre_aprovado ?? 0) > 0 && (
            <p>
              <span className="block text-4xl font-semibold tracking-tight tabular-nums">
                {formatarPercentualBps(visao.taxa_pre_aprovacao_bps)}
              </span>
              <span className="text-sm text-muted-foreground">pré-aprovadas nas consultas de limite</span>
            </p>
          )}
        </div>
        <div aria-hidden="true" className="mt-6 flex h-3 overflow-hidden rounded-full bg-secondary">
          {status.map((s) => (
            <span
              key={s}
              className={COR_BARRA[s]}
              style={{ width: `${proporcaoBps(visao.por_status[s] ?? 0, visao.total) / 100}%` }}
            />
          ))}
        </div>
        <ul className="mt-4 grid gap-3 sm:grid-cols-2">
          {status.map((s) => {
            const quantidade = visao.por_status[s] ?? 0
            return (
              <li
                key={s}
                className="flex items-center justify-between gap-3 rounded-xl bg-secondary px-3 py-2"
              >
                <StatusDecisao status={s} />
                <span className="text-sm tabular-nums">
                  <span className="font-semibold">{formatarInteiro(quantidade)}</span>{' '}
                  <span className="text-muted-foreground">
                    ({formatarPercentualBps(proporcaoBps(quantidade, visao.total))})
                  </span>
                </span>
              </li>
            )
          })}
        </ul>
      </Cartao>

      <Cartao aria-labelledby="titulo-sinais">
        <TituloCartao id="titulo-sinais" descricao="Os sinais de antifraude que mais apareceram.">
          Sinais frequentes
        </TituloCartao>
        {visao.sinais_frequentes.length === 0 ? (
          <p className="text-sm text-muted-foreground">Nenhum sinal no período.</p>
        ) : (
          <ul className="space-y-3">
            {visao.sinais_frequentes.map(({ sinal, quantidade }) => (
              <li key={sinal} className="space-y-1.5">
                <div className="flex items-center justify-between gap-2">
                  <Etiqueta tom="alerta">{rotuloSinal(sinal)}</Etiqueta>
                  <span className="text-sm font-semibold tabular-nums">{formatarInteiro(quantidade)}</span>
                </div>
                <div aria-hidden="true" className="h-1.5 rounded-full bg-secondary">
                  <div
                    className="h-full rounded-full bg-foreground/70"
                    style={{ width: `${(quantidade / maiorSinal) * 100}%` }}
                  />
                </div>
              </li>
            ))}
          </ul>
        )}
      </Cartao>

      <Cartao aria-labelledby="titulo-score" className="lg:col-span-3">
        <TituloCartao
          id="titulo-score"
          descricao="Quantas consultas caíram em cada faixa de score do bureau."
        >
          Distribuição de score
        </TituloCartao>
        <ol className="flex h-48 items-end gap-2 sm:gap-3">
          {visao.distribuicao_score.map(({ de, ate, quantidade }) => (
            <li key={de} className="flex h-full min-w-0 flex-1 flex-col items-center justify-end gap-1.5">
              <span className="text-xs font-semibold tabular-nums">{formatarInteiro(quantidade)}</span>
              <span
                aria-hidden="true"
                className="w-full rounded-t-lg bg-primary"
                style={{ height: `${Math.max((quantidade / maiorFaixa) * 100, 2)}%` }}
              />
              <span className="text-center text-[11px] leading-tight text-muted-foreground tabular-nums">
                <span className="sr-only">score de </span>
                {de}–<wbr />
                {ate}
              </span>
            </li>
          ))}
        </ol>
      </Cartao>
    </div>
  )
}
