import { apiClient } from '@/api/apiClient'
import { endpoints } from '@/api/endpoints'
import type { ApiPaths, Schemas } from '@/shared/types'

export type RegistroAuditoria = Schemas['RegistroAuditoria']
export type PaginaAuditoria =
  ApiPaths['/painel/auditoria']['get']['responses']['200']['content']['application/json']
export type Integridade =
  ApiPaths['/painel/auditoria/integridade']['get']['responses']['200']['content']['application/json']

export const TAMANHO_PAGINA = 50

export async function listarAuditoria(cursor?: string): Promise<PaginaAuditoria> {
  const { data } = await apiClient.get<PaginaAuditoria>(endpoints.auditoria.lista, {
    params: { cursor, limite: TAMANHO_PAGINA },
  })
  return data
}

export async function verificarIntegridade(): Promise<Integridade> {
  const { data } = await apiClient.get<Integridade>(endpoints.auditoria.integridade)
  return data
}
