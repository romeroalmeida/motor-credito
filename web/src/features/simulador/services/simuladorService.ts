import { apiClient } from '@/api/apiClient'
import { endpoints } from '@/api/endpoints'
import type { Schemas } from '@/shared/types'

export type ResultadoSimulador = Schemas['ResultadoSimulador']
export type RequisicaoConsulta = Schemas['ConsultaRequisicao']
export type ChaveAtiva = Pick<Schemas['Chave'], 'id' | 'nome' | 'prefixo'>

export interface EntradaSimulador {
  chave_id: string
  /** Vazia: o servidor gera um UUID novo. */
  idempotency_key?: string
  requisicao: RequisicaoConsulta
}

// Serviço próprio (sem importar a feature chaves): o simulador só precisa das ativas, sem segredo.
export async function listarChavesAtivas(): Promise<ChaveAtiva[]> {
  const { data } = await apiClient.get<{ itens: Schemas['Chave'][] }>(endpoints.chaves.lista)
  return data.itens
    .filter((chave) => chave.revogada_em === null)
    .map(({ id, nome, prefixo }) => ({ id, nome, prefixo }))
}

export async function simular(entrada: EntradaSimulador): Promise<ResultadoSimulador> {
  const { data } = await apiClient.post<ResultadoSimulador>(endpoints.simulador, entrada)
  return data
}
