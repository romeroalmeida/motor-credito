/// <reference types="vitest/config" />
import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// Padrão: a API do compose. `make dev` aponta para a API rodando no host.
const API_DEV = process.env.API_DEV ?? 'http://127.0.0.1:58080'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { '@': path.resolve(import.meta.dirname, './src') },
  },
  server: {
    port: 5179,
    strictPort: true,
    // Em dev sem mocks, o painel fala com a API do compose pelo mesmo host (cookies SameSite=Strict).
    proxy: {
      '/painel': API_DEV,
      '/v1': API_DEV,
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/tests/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
  },
})
