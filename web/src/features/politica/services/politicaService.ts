import { apiClient } from '@/api/apiClient'
import { endpoints } from '@/api/endpoints'
import type { ApiPaths, Schemas } from '@/shared/types'

export type PoliticaEstado = Schemas['PoliticaEstado']
export type PoliticaVersao = Schemas['PoliticaVersao']
export type PoliticaVersaoResumo = Schemas['PoliticaVersaoResumo']
export type ResultadoTeste = Schemas['ResultadoTeste']
export type Mudanca = Schemas['Mudanca']
export type PedidoTeste =
  ApiPaths['/painel/politicas/testar']['post']['requestBody']['content']['application/json']
type Diferenca =
  ApiPaths['/painel/politicas/diferenca']['get']['responses']['200']['content']['application/json']

export async function buscarEstado(): Promise<PoliticaEstado> {
  const { data } = await apiClient.get<PoliticaEstado>(endpoints.politicas.atual)
  return data
}

export async function salvarRascunho(politica: Schemas['Politica']): Promise<PoliticaEstado> {
  const { data } = await apiClient.put<PoliticaEstado>(endpoints.politicas.rascunho, politica)
  return data
}

export async function testarPolitica(pedido: PedidoTeste): Promise<ResultadoTeste> {
  const { data } = await apiClient.post<ResultadoTeste>(endpoints.politicas.testar, pedido)
  return data
}

export async function publicarRascunho(comentario: string): Promise<PoliticaVersao> {
  const { data } = await apiClient.post<PoliticaVersao>(endpoints.politicas.publicar, {
    confirmacao: true,
    comentario: comentario || undefined,
  })
  return data
}

export async function listarVersoes(): Promise<PoliticaVersaoResumo[]> {
  const { data } = await apiClient.get<{ itens: PoliticaVersaoResumo[] }>(endpoints.politicas.versoes)
  return data.itens
}

export async function reverterVersao(versao: number): Promise<PoliticaVersao> {
  const { data } = await apiClient.post<PoliticaVersao>(endpoints.politicas.reverter(versao), {
    confirmacao: true,
  })
  return data
}

export async function buscarDiferenca(de: number, para: string): Promise<Diferenca> {
  const { data } = await apiClient.get<Diferenca>(endpoints.politicas.diferenca, { params: { de, para } })
  return data
}
