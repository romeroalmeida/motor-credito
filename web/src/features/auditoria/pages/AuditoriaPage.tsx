import { ScrollText, ShieldCheck } from 'lucide-react'
import {
  AvisoErro,
  CabecalhoPagina,
  Carregando,
  Cartao,
  ErroCarregar,
  Etiqueta,
  Vazio,
} from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { lerErroApi } from '@/shared/utils/erroApi'
import { formatarDataHora } from '@/shared/utils/formato'
import { useIntegridade, useRegistrosAuditoria } from '../hooks/useAuditoria'
import type { RegistroAuditoria } from '../services/auditoriaService'

const ROTULO_EVENTO: Record<string, string> = {
  'consulta.decidida': 'Consulta decidida',
  'politica.publicada': 'Política publicada',
}

export function AuditoriaPage() {
  const registros = useRegistrosAuditoria()
  const integridade = useIntegridade()
  const itens = registros.data?.pages.flatMap((pagina) => pagina.itens) ?? []

  return (
    <>
      <CabecalhoPagina
        titulo="Auditoria"
        descricao="Cada decisão e cada publicação de política, encadeadas por hash. Só leitura: ninguém altera um registro sem quebrar a cadeia."
        acoes={
          <Button variant="outline" onClick={() => integridade.refetch()} disabled={integridade.isFetching}>
            <ShieldCheck aria-hidden="true" />
            {integridade.isFetching ? 'Verificando…' : 'Verificar integridade'}
          </Button>
        }
      />
      <ResultadoIntegridade integridade={integridade} />
      <Cartao aria-labelledby="titulo-registros" className="p-2 md:p-3">
        <h2 id="titulo-registros" className="sr-only">
          Registros
        </h2>
        {registros.isPending ? (
          <Carregando linhas={6} className="p-3" />
        ) : registros.isError ? (
          <ErroCarregar
            mensagem={lerErroApi(registros.error).mensagem}
            aoTentarDeNovo={() => registros.refetch()}
          />
        ) : itens.length === 0 ? (
          <Vazio icone={ScrollText} titulo="Nenhum registro ainda">
            Os registros aparecem a partir da primeira consulta ou publicação.
          </Vazio>
        ) : (
          <>
            <ul className="divide-y divide-border">
              {itens.map((registro) => (
                <ItemRegistro key={registro.seq} registro={registro} />
              ))}
            </ul>
            {registros.hasNextPage && (
              <div className="flex justify-center p-3">
                <Button
                  variant="outline"
                  onClick={() => registros.fetchNextPage()}
                  disabled={registros.isFetchingNextPage}
                >
                  {registros.isFetchingNextPage ? 'Carregando…' : 'Carregar mais'}
                </Button>
              </div>
            )}
          </>
        )}
      </Cartao>
    </>
  )
}

function ResultadoIntegridade({ integridade }: { integridade: ReturnType<typeof useIntegridade> }) {
  if (integridade.isError) return <AvisoErro>{lerErroApi(integridade.error).mensagem}</AvisoErro>
  if (!integridade.data || integridade.isFetching) return null
  return (
    <Cartao role="status" className="flex flex-wrap items-center gap-3">
      {integridade.data.integra ? (
        <Etiqueta tom="aprovado">Cadeia íntegra</Etiqueta>
      ) : (
        <>
          <Etiqueta tom="negado">Cadeia quebrada</Etiqueta>
          <span className="text-sm">{integridade.data.mensagem}</span>
        </>
      )}
    </Cartao>
  )
}

function ItemRegistro({ registro }: { registro: RegistroAuditoria }) {
  return (
    <li className="grid grid-cols-[auto_1fr_auto] items-center gap-x-4 gap-y-1 px-4 py-3 text-sm">
      <span className="font-semibold tabular-nums">
        <span className="sr-only">Sequência </span>#{registro.seq}
      </span>
      <div className="min-w-0">
        <p>{ROTULO_EVENTO[registro.evento] ?? registro.evento}</p>
        <p className="truncate font-mono text-xs text-muted-foreground">
          {registro.ator}
          {registro.consulta_id && ` · ${registro.consulta_id}`}
        </p>
      </div>
      <div className="flex flex-wrap items-center justify-end gap-2">
        <span className="text-muted-foreground tabular-nums">{formatarDataHora(registro.criado_em)}</span>
        {registro.hash_ok ? (
          <Etiqueta tom="aprovado">Hash confere</Etiqueta>
        ) : (
          <Etiqueta tom="negado">Hash não confere</Etiqueta>
        )}
      </div>
    </li>
  )
}
