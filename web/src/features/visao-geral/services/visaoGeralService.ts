import { apiClient } from '@/api/apiClient'
import { endpoints } from '@/api/endpoints'
import type { Schemas } from '@/shared/types'

export type VisaoGeral = Schemas['VisaoGeral']
export type Periodo = VisaoGeral['periodo']

export async function buscarVisaoGeral(periodo: Periodo): Promise<VisaoGeral> {
  const { data } = await apiClient.get<VisaoGeral>(endpoints.visaoGeral, { params: { periodo } })
  return data
}
