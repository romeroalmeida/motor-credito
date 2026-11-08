import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { server } from '@/mocks/server'
import { useSessaoStore } from '@/state/useSessaoStore'

// Requisição sem handler quebra o teste: nada vaza para a rede.
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  cleanup()
  useSessaoStore.setState({ eu: null })
  sessionStorage.clear()
})
afterAll(() => server.close())
