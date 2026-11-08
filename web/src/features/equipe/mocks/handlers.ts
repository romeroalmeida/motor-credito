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

const naoEncontrado = () => HttpResponse.json({ erro: 'nao_encontrado' }, { status: 404 })
const ultimoAdmin = () =>
  HttpResponse.json(
    { erro: 'ultimo_admin', mensagem: 'o tenant precisa de ao menos um admin' },
    { status: 409 },
  )
const unicoAdmin = (m: Schemas['Membro']) =>
  m.papel === 'admin' && membros.filter((x) => x.papel === 'admin').length <= 1

export const handlers: HttpHandler[] = [
  // Como no servidor: não rebaixa nem remove o último admin.
  http.patch('*/painel/equipe/:id', async ({ params, request }) => {
    const { papel } = (await request.json()) as { papel: Papel }
    await delay()
    const membro = membros.find((m) => m.usuario_id === params.id)
    if (!membro) return naoEncontrado()
    if (papel !== 'admin' && unicoAdmin(membro)) return ultimoAdmin()
    membro.papel = papel
    return HttpResponse.json(membro)
  }),

  http.delete('*/painel/equipe/:id', async ({ params }) => {
    await delay()
    const i = membros.findIndex((m) => m.usuario_id === params.id)
    const membro = membros[i]
    if (!membro) return naoEncontrado()
    if (unicoAdmin(membro)) return ultimoAdmin()
    membros.splice(i, 1)
    return new HttpResponse(null, { status: 204 })
  }),

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
