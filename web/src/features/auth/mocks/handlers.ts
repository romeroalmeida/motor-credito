import { delay, type HttpHandler, http, HttpResponse } from 'msw'
import type { Papel, Schemas } from '@/shared/types'

// Handlers MSW da feature, montados em src/mocks/handlers.ts (dev e Vitest).
// Demonstração: senha "senha-demo", código "123456"; o prefixo do e-mail escolhe o papel.

export const SENHA_DEMO = 'senha-demo'
export const CODIGO_DEMO = '123456'
export const TOKEN_CONVITE_DEMO = 'convite-demo'

// Sessão concluída sobrevive a recarregar a página (só no mock; a real vive no cookie httpOnly).
const CHAVE_SESSAO = 'mock-sessao-email'

const pendente = { email: '' }

function papelDoEmail(email: string): Papel {
  if (email.startsWith('analista@')) return 'analista'
  if (email.startsWith('auditor@')) return 'auditor'
  return 'admin'
}

function euDemo(email: string): Schemas['Eu'] {
  return {
    usuario: { id: 'usr_01JC8ZK3Q0ANA00000000000', nome: 'Ana Ribeiro', email },
    tenant: { id: 'ten_01JC8ZK3Q0CREDIFACIL0000', nome: 'CrediFácil' },
    papel: papelDoEmail(email),
  }
}

const naoAutorizado = () => HttpResponse.json({ erro: 'nao_autorizado' }, { status: 401 })

export const handlers: HttpHandler[] = [
  http.post('*/painel/auth/entrar', async ({ request }) => {
    const corpo = (await request.json()) as { email?: string; senha?: string }
    await delay()
    if (!corpo.email || corpo.senha !== SENHA_DEMO) return naoAutorizado()
    pendente.email = corpo.email
    document.cookie = 'csrf=csrf-demo; path=/'
    return HttpResponse.json({ proximo: '2fa' })
  }),

  http.post('*/painel/auth/cadastro', async ({ request }) => {
    const corpo = (await request.json()) as { token?: string }
    await delay()
    if (corpo.token !== TOKEN_CONVITE_DEMO)
      return HttpResponse.json({ erro: 'nao_encontrado' }, { status: 404 })
    pendente.email = 'convidada@credifacil.com.br'
    document.cookie = 'csrf=csrf-demo; path=/'
    return HttpResponse.json({
      proximo: '2fa',
      otpauth_url:
        'otpauth://totp/Motor%20de%20Cr%C3%A9dito:convidada?secret=JBSWY3DPEHPK3PXP&issuer=Motor%20de%20Cr%C3%A9dito',
      segredo_totp: 'JBSWY3DPEHPK3PXP',
    })
  }),

  http.post('*/painel/auth/2fa', async ({ request }) => {
    const corpo = (await request.json()) as { codigo?: string }
    await delay()
    if (!pendente.email || corpo.codigo !== CODIGO_DEMO) return naoAutorizado()
    sessionStorage.setItem(CHAVE_SESSAO, pendente.email)
    pendente.email = ''
    return HttpResponse.json(euDemo(sessionStorage.getItem(CHAVE_SESSAO) ?? ''))
  }),

  http.post('*/painel/auth/sair', () => {
    sessionStorage.removeItem(CHAVE_SESSAO)
    return new HttpResponse(null, { status: 204 })
  }),

  http.get('*/painel/auth/eu', () => {
    const email = sessionStorage.getItem(CHAVE_SESSAO)
    return email ? HttpResponse.json(euDemo(email)) : naoAutorizado()
  }),
]
