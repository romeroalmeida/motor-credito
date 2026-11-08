import { apiClient } from '@/api/apiClient'
import { endpoints } from '@/api/endpoints'
import type { Schemas } from '@/shared/types'

export type Chave = Schemas['Chave']
export type ChaveCriada = Schemas['ChaveCriada']

export async function listarChaves(): Promise<Chave[]> {
  const { data } = await apiClient.get<{ itens: Chave[] }>(endpoints.chaves.lista)
  return data.itens
}

export async function criarChave(nome: string): Promise<ChaveCriada> {
  const { data } = await apiClient.post<ChaveCriada>(endpoints.chaves.lista, { nome: nome || undefined })
  return data
}

export async function girarChave(id: string): Promise<ChaveCriada> {
  const { data } = await apiClient.post<ChaveCriada>(endpoints.chaves.girar(id))
  return data
}

export async function revogarChave(id: string): Promise<void> {
  await apiClient.delete(endpoints.chaves.chave(id))
}

export async function buscarIps(): Promise<string[]> {
  const { data } = await apiClient.get<Schemas['IPsPermitidos']>(endpoints.chaves.ips)
  return data.ips
}

export async function salvarIps(ips: string[]): Promise<string[]> {
  const { data } = await apiClient.put<Schemas['IPsPermitidos']>(endpoints.chaves.ips, { ips })
  return data.ips
}
