import { handlers as auditoria } from '@/features/auditoria/mocks/handlers'
import { handlers as auth } from '@/features/auth/mocks/handlers'
import { handlers as chaves } from '@/features/chaves/mocks/handlers'
import { handlers as consultas } from '@/features/consultas/mocks/handlers'
import { handlers as equipe } from '@/features/equipe/mocks/handlers'
import { handlers as politica } from '@/features/politica/mocks/handlers'
import { handlers as simulador } from '@/features/simulador/mocks/handlers'
import { handlers as visaoGeral } from '@/features/visao-geral/mocks/handlers'

// Fica fora dos barrels de propósito: msw nunca entra no bundle de produção.
export const handlers = [
  ...auth,
  ...visaoGeral,
  ...consultas,
  ...politica,
  ...chaves,
  ...simulador,
  ...equipe,
  ...auditoria,
]
