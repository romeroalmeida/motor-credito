import { defineConfig, devices } from '@playwright/test'

// e2e fica com a tarefa E (contra a API real). Aqui só a configuração.
export default defineConfig({
  testDir: './e2e',
  use: { baseURL: 'http://127.0.0.1:55173', trace: 'on-first-retry' },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
