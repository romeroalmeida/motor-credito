import axios from 'axios'
import { useSessaoStore } from '@/state/useSessaoStore'
import { endpoints } from './endpoints'

const METODOS_QUE_ESCREVEM = new Set(['post', 'put', 'patch', 'delete'])

// 401 nestas rotas é "senha/código errado", não "sessão caiu": a tela de Entrar trata.
const ROTAS_DE_LOGIN = new Set<string>([endpoints.auth.entrar, endpoints.auth.doisFatores])

// Double submit: o servidor compara o cabeçalho X-CSRF com o cookie `csrf` (legível pelo JS).
export function lerCookie(nome: string, cookies: string = document.cookie): string | undefined {
  const par = cookies.split('; ').find((item) => item.startsWith(`${nome}=`))
  return par ? decodeURIComponent(par.slice(nome.length + 1)) : undefined
}

// Mesma origem (proxy do Vite / nginx): os cookies de sessão são SameSite=Strict.
export const apiClient = axios.create({
  withCredentials: true,
  headers: { 'Content-Type': 'application/json' },
})

apiClient.interceptors.request.use((config) => {
  const csrf = lerCookie('csrf')
  if (csrf && METODOS_QUE_ESCREVEM.has(config.method ?? '')) {
    config.headers.set('X-CSRF', csrf)
  }
  return config
})

// Sessão vencida em qualquer chamada: limpa o retrato da sessão e a guarda de rota manda para Entrar.
apiClient.interceptors.response.use(undefined, (erro: unknown) => {
  if (
    axios.isAxiosError(erro) &&
    erro.response?.status === 401 &&
    !ROTAS_DE_LOGIN.has(erro.config?.url ?? '')
  ) {
    useSessaoStore.getState().limpar()
  }
  return Promise.reject(erro)
})
