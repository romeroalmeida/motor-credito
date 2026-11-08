import { apiClient } from '@/api/apiClient'
import { endpoints } from '@/api/endpoints'
import type { ApiPaths, Papel, Schemas } from '@/shared/types'

export type Membro = Schemas['Membro']
export type Convite = Schemas['Convite']
type Equipe = ApiPaths['/painel/equipe']['get']['responses']['200']['content']['application/json']
export type ConviteCriado =
  ApiPaths['/painel/equipe']['post']['responses']['201']['content']['application/json']

export async function buscarEquipe(): Promise<Equipe> {
  const { data } = await apiClient.get<Equipe>(endpoints.equipe)
  return data
}

export async function convidar(convite: { email: string; papel: Papel }): Promise<ConviteCriado> {
  const { data } = await apiClient.post<ConviteCriado>(endpoints.equipe, convite)
  return data
}
