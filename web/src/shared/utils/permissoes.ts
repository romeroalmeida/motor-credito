import type { Papel } from '@/shared/types'

// Espelho dos papéis do contrato (api/openapi.yaml): o servidor é quem barra (403);
// aqui só escondemos o que o papel não pode usar, para a tela não oferecer beco sem saída.
export type Area = 'visao-geral' | 'consultas' | 'politica' | 'chaves' | 'equipe' | 'auditoria' | 'simulador'

const LEITURA: Record<Area, readonly Papel[]> = {
  'visao-geral': ['admin', 'analista'],
  consultas: ['admin', 'analista'],
  politica: ['admin', 'analista'],
  chaves: ['admin'],
  equipe: ['admin'],
  auditoria: ['admin', 'auditor'],
  simulador: ['admin'],
}

export function podeVer(papel: Papel | null, area: Area): boolean {
  return papel !== null && LEITURA[area].includes(papel)
}

/** Escrever (salvar, testar, publicar, criar, revogar, convidar) é só do admin em todas as áreas.
 * Ver a diferença entre versões vale para quem lê a política (admin e analista). */
export function podeEditar(papel: Papel | null): boolean {
  return papel === 'admin'
}

export const ROTULO_PAPEL: Record<Papel, string> = {
  admin: 'Administrador',
  analista: 'Analista',
  auditor: 'Auditor',
}
