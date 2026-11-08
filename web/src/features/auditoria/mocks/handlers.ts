import { delay, type HttpHandler, http, HttpResponse } from 'msw'
import type { Schemas } from '@/shared/types'

// Handlers MSW da feature, montados em src/mocks/handlers.ts (dev e Vitest).
// 60 registros; o seq 7 tem o hash adulterado para a tela mostrar o alerta.

type Registro = Schemas['RegistroAuditoria']

export const SEQ_ADULTERADO = 7
const TOTAL = 60

const REGISTROS: Registro[] = Array.from({ length: TOTAL }, (_, i) => {
  const seq = TOTAL - i
  const publicacao = seq % 10 === 1
  return {
    seq,
    evento: publicacao ? 'politica.publicada' : 'consulta.decidida',
    ator: publicacao ? 'usuario:usr_01JC8ZK3Q0ADMIN0000000000' : 'api:chv_01JC8ZK3Q0PRODUCAO00000',
    consulta_id: publicacao ? null : `cons_01JC8ZK3Q0${String(seq).padStart(14, '0')}`,
    criado_em: new Date(Date.UTC(2026, 10, 7, 12, 0, 0) - i * 60_000).toISOString(),
    hash_ok: seq !== SEQ_ADULTERADO,
  }
})

export const handlers: HttpHandler[] = [
  http.get('*/painel/auditoria', async ({ request }) => {
    const parametros = new URL(request.url).searchParams
    const limite = Number(parametros.get('limite') ?? 25)
    const antesDe = Number(parametros.get('cursor') ?? Infinity)
    const pagina = REGISTROS.filter((r) => r.seq < antesDe).slice(0, limite)
    const ultimo = pagina.at(-1)
    const proximo = pagina.length === limite && ultimo && ultimo.seq > 1 ? String(ultimo.seq) : null
    await delay()
    return HttpResponse.json({ itens: pagina, proximo_cursor: proximo })
  }),

  http.get('*/painel/auditoria/integridade', async () => {
    await delay()
    return HttpResponse.json({
      integra: false,
      mensagem: `cadeia de auditoria quebrada: seq ${SEQ_ADULTERADO} alterado`,
    })
  }),
]
