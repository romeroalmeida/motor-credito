import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { buscarConsulta, type FiltrosConsultas, listarConsultas } from '../services/consultasService'

export function useConsultas(filtros: FiltrosConsultas) {
  return useInfiniteQuery({
    queryKey: ['consultas', 'lista', filtros],
    queryFn: ({ pageParam }) => listarConsultas(filtros, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (pagina) => pagina.proximo_cursor ?? undefined,
  })
}

export function useConsulta(id: string) {
  return useQuery({ queryKey: ['consultas', 'detalhe', id], queryFn: () => buscarConsulta(id) })
}
