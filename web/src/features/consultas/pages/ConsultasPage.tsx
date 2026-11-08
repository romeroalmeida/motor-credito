import { ChevronRight, SearchX } from 'lucide-react'
import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { CabecalhoPagina, Carregando, Cartao, ErroCarregar, StatusDecisao, Vazio } from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import type { StatusDecisao as Status } from '@/shared/types'
import { lerErroApi } from '@/shared/utils/erroApi'
import { formatarCentavos, formatarDataHora } from '@/shared/utils/formato'
import { FiltrosConsultas } from '../components/FiltrosConsultas'
import { useConsultas } from '../hooks/useConsultas'
import type { ConsultaResumo, FiltrosConsultas as Filtros } from '../services/consultasService'

// Status, faixa e datas vão na URL (dá para compartilhar o link). O documento não: ficaria no histórico do navegador.
const FILTROS_NA_URL = ['status', 'faixa', 'desde', 'ate'] as const

function filtrosDaUrl(parametros: URLSearchParams): Omit<Filtros, 'documento'> {
  return {
    status: (parametros.get('status') as Status | null) ?? undefined,
    faixa: parametros.get('faixa') ?? undefined,
    desde: parametros.get('desde') ?? undefined,
    ate: parametros.get('ate') ?? undefined,
  }
}

export function ConsultasPage() {
  const [parametros, setParametros] = useSearchParams()
  const [documento, setDocumento] = useState<string>()
  const filtros: Filtros = { ...filtrosDaUrl(parametros), documento }
  const consultas = useConsultas(filtros)

  function aplicar({ documento: novoDocumento, ...resto }: Filtros) {
    setDocumento(novoDocumento)
    const novos = new URLSearchParams()
    for (const campo of FILTROS_NA_URL) {
      const valor = resto[campo]
      if (valor) novos.set(campo, valor)
    }
    setParametros(novos, { replace: true })
  }

  const itens = consultas.data?.pages.flatMap((pagina) => pagina.itens) ?? []

  return (
    <>
      <CabecalhoPagina
        titulo="Consultas"
        descricao="Cada decisão do motor, da mais recente para a mais antiga. O documento aparece sempre mascarado."
      />
      <Cartao aria-label="Filtros">
        <FiltrosConsultas filtros={filtros} aoMudar={aplicar} />
      </Cartao>
      <Cartao aria-labelledby="titulo-resultados" className="p-2 md:p-3">
        <h2 id="titulo-resultados" className="sr-only">
          Resultados
        </h2>
        {consultas.isPending ? (
          <Carregando linhas={6} className="p-3" />
        ) : consultas.isError ? (
          <ErroCarregar
            mensagem={lerErroApi(consultas.error).mensagem}
            aoTentarDeNovo={() => consultas.refetch()}
          />
        ) : itens.length === 0 ? (
          <Vazio icone={SearchX} titulo="Nenhuma consulta encontrada">
            Tente outro período ou limpe os filtros.
          </Vazio>
        ) : (
          <>
            <div
              aria-hidden="true"
              className="hidden grid-cols-[1.4fr_1.1fr_0.5fr_0.5fr_1fr_1fr_1.2rem] gap-4 px-4 pt-2 pb-3 text-xs font-medium text-muted-foreground lg:grid"
            >
              <span>Documento</span>
              <span>Status</span>
              <span>Score</span>
              <span>Faixa</span>
              <span>Pedido</span>
              <span>Data</span>
            </div>
            <ul className="divide-y divide-border">
              {itens.map((consulta) => (
                <ItemConsulta key={consulta.id} consulta={consulta} />
              ))}
            </ul>
            {consultas.hasNextPage && (
              <div className="flex justify-center p-3">
                <Button
                  variant="outline"
                  onClick={() => consultas.fetchNextPage()}
                  disabled={consultas.isFetchingNextPage}
                >
                  {consultas.isFetchingNextPage ? 'Carregando…' : 'Carregar mais'}
                </Button>
              </div>
            )}
          </>
        )}
      </Cartao>
    </>
  )
}

function ItemConsulta({ consulta }: { consulta: ConsultaResumo }) {
  return (
    <li>
      <Link
        to={`/consultas/${encodeURIComponent(consulta.id)}`}
        className="grid grid-cols-[1fr_auto] items-center gap-x-4 gap-y-1 rounded-xl px-4 py-3 transition-colors hover:bg-secondary lg:grid-cols-[1.4fr_1.1fr_0.5fr_0.5fr_1fr_1fr_1.2rem]"
      >
        <span className="font-mono text-sm">
          <span className="sr-only">Documento </span>
          {consulta.documento_mascarado}
        </span>
        <StatusDecisao status={consulta.status} className="justify-self-end lg:justify-self-start" />
        <span className="text-sm text-muted-foreground tabular-nums lg:text-foreground">
          <span className="lg:sr-only">Score </span>
          {consulta.score ?? '—'}
        </span>
        <span className="justify-self-end text-sm text-muted-foreground lg:justify-self-start lg:text-foreground">
          <span className="lg:sr-only">Faixa </span>
          {consulta.faixa_risco ?? '—'}
        </span>
        <span className="text-sm tabular-nums">
          <span className="sr-only">Pedido </span>
          {formatarCentavos(consulta.pedido.valor)}
          <span className="text-muted-foreground"> em {consulta.pedido.prazo}×</span>
        </span>
        <span className="justify-self-end text-sm text-muted-foreground tabular-nums lg:justify-self-start">
          {formatarDataHora(consulta.criado_em)}
        </span>
        <ChevronRight aria-hidden="true" className="hidden size-4 text-muted-foreground lg:block" />
      </Link>
    </li>
  )
}
