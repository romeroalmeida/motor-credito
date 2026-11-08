import { delay, type HttpHandler, http, HttpResponse } from 'msw'
import type { Schemas } from '@/shared/types'

// Handlers MSW da feature, montados em src/mocks/handlers.ts (dev e Vitest).

type VisaoGeral = Schemas['VisaoGeral']

function montar(periodo: VisaoGeral['periodo'], fator: number): VisaoGeral {
  const por_status = {
    aprovado: 58 * fator,
    contraproposta: 21 * fator,
    negado: 34 * fator,
    documento_invalido: 4 * fator,
  }
  const total = Object.values(por_status).reduce((a, b) => a + b, 0)
  return {
    periodo,
    total,
    por_status,
    taxa_aprovacao_bps: Math.round(((por_status.aprovado + por_status.contraproposta) * 10000) / total),
    distribuicao_score: [
      { de: 0, ate: 200, quantidade: 6 * fator },
      { de: 201, ate: 300, quantidade: 11 * fator },
      { de: 301, ate: 400, quantidade: 14 * fator },
      { de: 401, ate: 500, quantidade: 17 * fator },
      { de: 501, ate: 600, quantidade: 22 * fator },
      { de: 601, ate: 700, quantidade: 24 * fator },
      { de: 701, ate: 800, quantidade: 15 * fator },
      { de: 801, ate: 1000, quantidade: 8 * fator },
    ],
    sinais_frequentes: [
      { sinal: 'email_descartavel', quantidade: 9 * fator },
      { sinal: 'ip_datacenter', quantidade: 6 * fator },
      { sinal: 'velocidade_alta', quantidade: 4 * fator },
      { sinal: 'nome_divergente', quantidade: 2 * fator },
    ],
  }
}

const POR_PERIODO: Record<VisaoGeral['periodo'], VisaoGeral> = {
  hoje: montar('hoje', 1),
  '7d': montar('7d', 6),
  '30d': montar('30d', 25),
}

export const handlers: HttpHandler[] = [
  http.get('*/painel/visao-geral', async ({ request }) => {
    const periodo = new URL(request.url).searchParams.get('periodo') ?? 'hoje'
    await delay()
    const visao = POR_PERIODO[periodo as VisaoGeral['periodo']]
    return visao ? HttpResponse.json(visao) : HttpResponse.json({ erro: 'corpo_invalido' }, { status: 400 })
  }),
]
