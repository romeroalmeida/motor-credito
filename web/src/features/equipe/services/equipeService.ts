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

// Rota do membro montada aqui: endpoints.ts (pasta api) não é desta feature.
const membro = (usuarioId: string) => `${endpoints.equipe}/${encodeURIComponent(usuarioId)}`

export async function trocarPapel({
  usuarioId,
  papel,
}: {
  usuarioId: string
  papel: Papel
}): Promise<Membro> {
  const { data } = await apiClient.patch<Membro>(membro(usuarioId), { papel })
  return data
}

export async function removerMembro(usuarioId: string): Promise<void> {
  await apiClient.delete(membro(usuarioId))
}

export async function convidar(convite: { email: string; papel: Papel }): Promise<ConviteCriado> {
  const { data } = await apiClient.post<ConviteCriado>(endpoints.equipe, convite)
  return data
}
