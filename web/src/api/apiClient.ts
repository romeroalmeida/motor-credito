import axios from 'axios'

const METODOS_QUE_ESCREVEM = new Set(['post', 'put', 'patch', 'delete'])

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
