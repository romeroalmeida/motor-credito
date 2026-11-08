import { apiClient } from '@/api/apiClient'
import { endpoints } from '@/api/endpoints'
import type { ApiPaths, Schemas } from '@/shared/types'

export type Eu = Schemas['Eu']

export async function entrar(credenciais: {
  email: string
  senha: string
}): Promise<Schemas['ProximoPasso']> {
  const { data } = await apiClient.post<Schemas['ProximoPasso']>(endpoints.auth.entrar, credenciais)
  return data
}

export type Cadastro =
  ApiPaths['/painel/auth/cadastro']['post']['responses']['200']['content']['application/json']

export async function cadastrar(dados: { token: string; nome: string; senha: string }): Promise<Cadastro> {
  const { data } = await apiClient.post<Cadastro>(endpoints.auth.cadastro, dados)
  return data
}

export async function confirmarDoisFatores(codigo: string): Promise<Eu> {
  const { data } = await apiClient.post<Eu>(endpoints.auth.doisFatores, { codigo })
  return data
}

export async function sair(): Promise<void> {
  await apiClient.post(endpoints.auth.sair)
}

export async function buscarEu(): Promise<Eu> {
  const { data } = await apiClient.get<Eu>(endpoints.auth.eu)
  return data
}
