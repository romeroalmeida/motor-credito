import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { listarAuditoria, verificarIntegridade } from '../services/auditoriaService'

export function useRegistrosAuditoria() {
  return useInfiniteQuery({
    queryKey: ['auditoria', 'registros'],
    queryFn: ({ pageParam }) => listarAuditoria(pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (pagina) => pagina.proximo_cursor ?? undefined,
  })
}

/** Percorre a cadeia inteira: só roda quando a pessoa pede. */
export function useIntegridade() {
  return useQuery({
    queryKey: ['auditoria', 'integridade'],
    queryFn: verificarIntegridade,
    enabled: false,
    gcTime: 0,
  })
}
