import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { buscarVisaoGeral, type Periodo } from '../services/visaoGeralService'

export function useVisaoGeral(periodo: Periodo) {
  return useQuery({
    queryKey: ['visao-geral', periodo],
    queryFn: () => buscarVisaoGeral(periodo),
    placeholderData: keepPreviousData,
  })
}
