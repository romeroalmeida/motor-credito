import { http, HttpResponse } from 'msw'
import { server } from '@/mocks/server'
import { apiClient, lerCookie } from './apiClient'

describe('lerCookie', () => {
  it('acha o cookie pelo nome exato', () => {
    expect(lerCookie('csrf', 'xcsrf=errado; csrf=abc%3D')).toBe('abc=')
    expect(lerCookie('csrf', 'sessao=1')).toBeUndefined()
  })
})

describe('apiClient', () => {
  let cabecalho: string | null = null
  beforeEach(() => {
    document.cookie = 'csrf=token-1'
    server.use(
      http.all('*/painel/teste', ({ request }) => {
        cabecalho = request.headers.get('X-CSRF')
        return HttpResponse.json({})
      }),
    )
  })

  it('manda X-CSRF em método que escreve', async () => {
    await apiClient.post('/painel/teste', {})
    expect(cabecalho).toBe('token-1')
  })

  it('não manda X-CSRF em GET', async () => {
    await apiClient.get('/painel/teste')
    expect(cabecalho).toBeNull()
  })
})
