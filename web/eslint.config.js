import js from '@eslint/js'
import prettier from 'eslint-config-prettier'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import { defineConfig, globalIgnores } from 'eslint/config'
import globals from 'globals'
import tseslint from 'typescript-eslint'

export default defineConfig([
  globalIgnores(['dist', 'coverage', 'public/mockServiceWorker.js', 'src/shared/types/api.gen.ts']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.strict,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
      prettier,
    ],
    languageOptions: { globals: globals.browser },
    rules: {
      // Feature não importa de outra feature: só pelo barrel, e nunca de dentro de features/.
      'no-restricted-imports': [
        'error',
        {
          patterns: [{ group: ['@/features/*/*'], message: 'Importe a feature pelo barrel (@/features/x).' }],
        },
      ],
    },
  },
  {
    // O agregador de mocks lê os handlers de cada feature direto, fora dos barrels.
    files: ['src/mocks/**'],
    rules: { 'no-restricted-imports': 'off' },
  },
  {
    files: ['src/shared/components/ui/**'],
    rules: { 'react-refresh/only-export-components': 'off' },
  },
])
