import { delay, type HttpHandler, http, HttpResponse } from 'msw'
import type { Papel, Schemas } from '@/shared/types'

// Handlers MSW da feature, montados em src/mocks/handlers.ts (dev e Vitest).

const SETE_DIAS = 7 * 24 * 60 * 60 * 1000

const membros: Schemas['Membro'][] = [
  {
    usuario_id: 'usr_01JC8ZK3Q0ANA00000000000',
    nome: 'Ana Ribeiro',
    email: 'ana@credifacil.com.br',
    papel: 'admin',
    desde: '2026-09-01T12:00:00Z',
  },
  {
    usuario_id: 'usr_01JC8ZK3Q0BRUNO000000000',
    nome: 'Bruno Costa',
    email: 'bruno@credifacil.com.br',
    papel: 'analista',
    desde: '2026-09-12T15:30:00Z',
  },
  {
    usuario_id: 'usr_01JC8ZK3Q0CARLA000000000',
    nome: 'Carla Mendes',
    email: 'carla@credifacil.com.br',
    papel: 'auditor',
    desde: '2026-10-20T10:00:00Z',
  },
]

const convites: Schemas['Convite'][] = [
  {
    id: 'cvt_01JC8ZK3Q0DIEGO000000000',
    email: 'diego@credifacil.com.br',
    papel: 'analista',
    criado_em: '2026-11-05T14:00:00Z',
    expira_em: '2026-11-12T14:00:00Z',
  },
]

export const handlers: HttpHandler[] = [
  http.get('*/painel/equipe', async () => {
    await delay()
    return HttpResponse.json({ membros, convites })
  }),

  http.post('*/painel/equipe', async ({ request }) => {
    const { email, papel } = (await request.json()) as { email: string; papel: Papel }
    await delay()
    const existe = [...membros, ...convites].some((p) => p.email.toLowerCase() === email.toLowerCase())
    if (existe) return HttpResponse.json({ erro: 'conflito' }, { status: 409 })
    const agora = Date.now()
    const convite: Schemas['Convite'] = {
      id: `cvt_01JC8ZK3Q0NOVO${String(convites.length).padStart(10, '0')}`,
      email,
      papel,
      criado_em: new Date(agora).toISOString(),
      expira_em: new Date(agora + SETE_DIAS).toISOString(),
    }
    convites.push(convite)
    return HttpResponse.json(
      { ...convite, link: `${location.origin}/cadastro?token=demo-${convite.id}` },
      { status: 201 },
    )
  }),
]
