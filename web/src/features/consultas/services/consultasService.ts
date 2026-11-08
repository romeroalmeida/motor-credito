import { apiClient } from '@/api/apiClient'
import { endpoints } from '@/api/endpoints'
import type { ApiPaths, Schemas } from '@/shared/types'

export type ConsultaResumo = Schemas['ConsultaResumo']
export type ConsultaDetalhe = Schemas['ConsultaDetalhe']
export type PaginaConsultas =
  ApiPaths['/painel/consultas/busca']['post']['responses']['200']['content']['application/json']

export interface FiltrosConsultas {
  status?: Schemas['StatusDecisao']
  faixa?: string
  /** Documento completo e normalizado; o servidor busca por HMAC, nunca por parte. */
  documento?: string
  desde?: string
  ate?: string
}

export const TAMANHO_PAGINA = 25

/** POST com os filtros no corpo: o documento nunca vai na URL (log de acesso). */
export async function listarConsultas(filtros: FiltrosConsultas, cursor?: string): Promise<PaginaConsultas> {
  const { data } = await apiClient.post<PaginaConsultas>(endpoints.consultas.busca, {
    ...filtros,
    cursor,
    limite: TAMANHO_PAGINA,
  })
  return data
}

export async function buscarConsulta(id: string): Promise<ConsultaDetalhe> {
  const { data } = await apiClient.get<ConsultaDetalhe>(endpoints.consultas.detalhe(id))
  return data
}
