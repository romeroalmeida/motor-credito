import { Cartao, TituloCartao } from '@/shared/components'
import { cn } from '@/shared/lib/utils'
import type { ResultadoSimulador } from '../services/simuladorService'
import { BlocoCodigo, BotaoCopiar } from './BotaoCopiar'
import { jsonLegivel } from '../utils/jsonLegivel'

const ORDEM = ['Authorization', 'Content-Type', 'X-Timestamp', 'Idempotency-Key', 'X-Assinatura']

export function CartaoRequisicao({ requisicao }: { requisicao: ResultadoSimulador['requisicao'] }) {
  const cabecalhos = Object.entries(requisicao.cabecalhos).sort(
    ([a], [b]) => ORDEM.indexOf(a) - ORDEM.indexOf(b),
  )
  return (
    <Cartao aria-labelledby="titulo-requisicao">
      <TituloCartao
        id="titulo-requisicao"
        descricao="O que o servidor enviou à API pública, como seu sistema enviaria."
      >
        Requisição
      </TituloCartao>
      <p className="mb-4 flex min-w-0 items-baseline gap-2 font-mono text-sm">
        <span className="shrink-0 rounded-md bg-primary px-1.5 py-0.5 text-xs font-semibold text-primary-foreground">
          {requisicao.metodo}
        </span>
        <span className="min-w-0 break-all">{requisicao.url}</span>
      </p>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <caption className="sr-only">Cabeçalhos enviados</caption>
          <thead className="text-xs text-muted-foreground">
            <tr>
              <th scope="col" className="py-2 pr-4 font-medium">
                Cabeçalho
              </th>
              <th scope="col" className="py-2 font-medium">
                Valor
              </th>
            </tr>
          </thead>
          <tbody>
            {cabecalhos.map(([nome, valor]) => {
              const assinatura = nome === 'X-Assinatura'
              return (
                <tr key={nome} className={cn('align-top', assinatura && 'bg-secondary')}>
                  <th
                    scope="row"
                    className={cn(
                      'py-2 pr-4 font-mono text-xs font-medium whitespace-nowrap',
                      assinatura && 'pl-2',
                    )}
                  >
                    {nome}
                  </th>
                  <td className="py-2 pr-2 font-mono text-xs break-all">{valor}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      <p className="mt-3 text-xs text-muted-foreground">
        <strong className="font-semibold text-foreground">X-Assinatura</strong> = hex(HMAC-SHA256(segredo,{' '}
        <code className="font-mono">X-Timestamp + "." + Idempotency-Key + "." + corpo</code>)). O segredo da
        chave só assina: nunca vai em cabeçalho, e não aparece aqui.
      </p>
      <div className="mt-5 space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-semibold">Corpo</h3>
          <BotaoCopiar texto={requisicao.corpo} rotulo="Copiar corpo da requisição" />
        </div>
        <BlocoCodigo codigo={jsonLegivel(requisicao.corpo)} rotulo="Corpo da requisição em JSON" />
        <p className="text-xs text-muted-foreground">
          A assinatura cobre o texto exato, compacto (é o que o botão copia); a quebra de linha é só para
          leitura.
        </p>
      </div>
    </Cartao>
  )
}
