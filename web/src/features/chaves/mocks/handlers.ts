import { delay, type HttpHandler, http, HttpResponse } from 'msw'
import type { Schemas } from '@/shared/types'

// Handlers MSW da feature, montados em src/mocks/handlers.ts (dev e Vitest).
// Como no servidor: o segredo só existe na resposta de criar/girar; a lista guarda o prefixo.

type Chave = Schemas['Chave']

const chaves: Chave[] = [
  {
    id: 'chv_01JC8ZK3Q0PRODUCAO000000',
    nome: 'produção',
    prefixo: 'ck_live_chv_01JC8ZK3Q0PRODUCAO000000_…',
    criada_em: '2026-10-02T12:10:00Z',
    revogada_em: null,
    ultimo_uso_em: '2026-11-07T13:58:41Z',
  },
  {
    id: 'chv_01JC8ZK3Q0HOMOLOGACAO000',
    nome: 'homologação',
    prefixo: 'ck_live_chv_01JC8ZK3Q0HOMOLOGACAO000_…',
    criada_em: '2026-09-15T09:00:00Z',
    revogada_em: '2026-10-01T18:30:00Z',
    ultimo_uso_em: '2026-09-30T22:14:00Z',
  },
]

let ips = ['200.150.10.0/24', '177.70.4.12']
let sequencia = 0

function gerar(nome: string): Schemas['ChaveCriada'] {
  sequencia += 1
  const id = `chv_01JC8ZK3Q0NOVA${String(sequencia).padStart(10, '0')}`
  chaves.unshift({
    id,
    nome,
    prefixo: `ck_live_${id}_…`,
    criada_em: new Date().toISOString(),
    revogada_em: null,
    ultimo_uso_em: null,
  })
  return { id, segredo: `ck_live_${id}_s3gr3d0demo${sequencia}x9Qm2LkP7vZt4RwN8aYc` }
}

const ativa = (id: unknown) => chaves.find((c) => c.id === id && c.revogada_em === null)
const naoEncontrado = () => HttpResponse.json({ erro: 'nao_encontrado' }, { status: 404 })

export const handlers: HttpHandler[] = [
  http.get('*/painel/chaves', async () => {
    await delay()
    return HttpResponse.json({ itens: chaves })
  }),

  http.post('*/painel/chaves', async ({ request }) => {
    const corpo = (await request.json().catch(() => ({}))) as { nome?: string }
    await delay()
    return HttpResponse.json(gerar(corpo.nome ?? ''), { status: 201 })
  }),

  http.delete('*/painel/chaves/:id', async ({ params }) => {
    const chave = ativa(params.id)
    await delay()
    if (!chave) return naoEncontrado()
    chave.revogada_em = new Date().toISOString()
    return new HttpResponse(null, { status: 204 })
  }),

  http.post('*/painel/chaves/:id/girar', async ({ params }) => {
    const chave = ativa(params.id)
    await delay()
    if (!chave) return naoEncontrado()
    chave.revogada_em = new Date().toISOString()
    return HttpResponse.json(gerar(chave.nome), { status: 201 })
  }),

  http.get('*/painel/chaves/ips', async () => {
    await delay()
    return HttpResponse.json({ ips })
  }),

  http.put('*/painel/chaves/ips', async ({ request }) => {
    const corpo = (await request.json()) as Schemas['IPsPermitidos']
    await delay()
    ips = corpo.ips
    return HttpResponse.json({ ips })
  }),
]
