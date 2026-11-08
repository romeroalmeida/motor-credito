import { delay, type HttpHandler, http, HttpResponse } from 'msw'
import type { Schemas, StatusDecisao } from '@/shared/types'
import type { EntradaSimulador, ResultadoSimulador } from '../services/simuladorService'

// Handler MSW da feature, montado em src/mocks/handlers.ts (dev e Vitest). As chaves vêm do mock da feature chaves.
// Imita o servidor: o segredo nunca sai dele (aqui ele nem existe; a assinatura é um valor fixo de mentira).

/** Chave revogada do mock de chaves: o servidor responde 404 para ela. */
const CHAVE_REVOGADA = '01JC8ZK3Q0HOMOLOGACAO00000'
const ASSINATURA_FALSA = 'f'.repeat(64)

function decidir(documento: string, valor: number): StatusDecisao {
  const doc = documento.replace(/\D/g, '')
  if (doc === '00000000353') return 'negado'
  if (doc === '00000000191' || doc === '12345678900') return 'documento_invalido'
  if (doc === '00000000434' && valor > 2_000_000) return 'contraproposta'
  return 'aprovado'
}

const porIdem = new Map<string, ResultadoSimulador>()
let sequencia = 0

function executar(entrada: EntradaSimulador): ResultadoSimulador {
  sequencia += 1
  const idem = entrada.idempotency_key || `00000000-0000-4000-8000-${String(sequencia).padStart(12, '0')}`
  const corpo = JSON.stringify(entrada.requisicao)
  const guardado = porIdem.get(idem)
  if (guardado && guardado.requisicao.corpo === corpo) return guardado
  const { documento, pedido } = entrada.requisicao
  const id = `cons_01JCSIMULADOR${String(sequencia).padStart(12, '0')}`
  const decisao: Schemas['ConsultaResposta'] = {
    id,
    status: decidir(documento, pedido.valor),
    pedido,
    motivos: [],
    criado_em: new Date().toISOString(),
    fontes: { cadastro: 'simulado', score: 'simulado', restricoes: 'simulado' },
  }
  const url = 'http://localhost:55173/v1/consultas'
  const resultado: ResultadoSimulador = {
    requisicao: {
      metodo: 'POST',
      url,
      cabecalhos: {
        Authorization: `Bearer ck_live_${entrada.chave_id}`,
        'Content-Type': 'application/json',
        'X-Timestamp': String(Math.floor(Date.now() / 1000)),
        'Idempotency-Key': idem,
        'X-Assinatura': ASSINATURA_FALSA,
      },
      corpo,
    },
    resposta: {
      status: 200,
      cabecalhos: { 'Content-Type': 'application/json' },
      corpo: decisao,
      duracao_ms: 42,
    },
    consulta_id: id,
    trechos: {
      curl: `# Defina MOTOR_SEGREDO com o segredo mostrado na criação da chave.\nCORPO='${corpo}'\ncurl -sS -X POST '${url}' --data-raw "$CORPO"`,
      node: `// Node 18+. Defina MOTOR_SEGREDO.\nconst corpo = ${JSON.stringify(corpo)}\ncreateHmac('sha256', process.env.MOTOR_SEGREDO)`,
      python: `# Python 3. Defina MOTOR_SEGREDO.\ncorpo = ${JSON.stringify(corpo)}\nos.environ["MOTOR_SEGREDO"]`,
    },
  }
  porIdem.set(idem, resultado)
  return resultado
}

export const handlers: HttpHandler[] = [
  http.post('*/painel/simulador/consultas', async ({ request }) => {
    const entrada = (await request.json()) as EntradaSimulador
    await delay()
    if (entrada.chave_id === CHAVE_REVOGADA) {
      return HttpResponse.json({ erro: 'nao_encontrado' }, { status: 404 })
    }
    return HttpResponse.json(executar(entrada))
  }),
]
