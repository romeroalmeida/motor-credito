import './configurarZod' // antes de tudo: os esquemas do zod são criados ao importar os módulos
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from '@/app/App'
import '@/shared/styles/index.css'

async function ligarMocks() {
  if (!import.meta.env.DEV || import.meta.env.VITE_USAR_MOCKS !== 'on') return
  // Import dinâmico: msw fica fora do bundle de produção.
  const { worker } = await import('@/mocks/browser')
  await worker.start({ onUnhandledFrame: 'bypass' })
}

const raiz = document.getElementById('root')
if (!raiz) throw new Error('elemento #root ausente no index.html')

await ligarMocks()
createRoot(raiz).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
