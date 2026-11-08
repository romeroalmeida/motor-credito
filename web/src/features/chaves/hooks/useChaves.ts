import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  buscarIps,
  criarChave,
  girarChave,
  listarChaves,
  revogarChave,
  salvarIps,
} from '../services/chavesService'

const CHAVE_LISTA = ['chaves', 'lista'] as const
const CHAVE_IPS = ['chaves', 'ips'] as const

export function useChaves() {
  return useQuery({ queryKey: CHAVE_LISTA, queryFn: listarChaves })
}

// O segredo volta só na resposta da mutação: nunca entra no cache de consultas.
function useMutacaoDeChave<T, R>(fn: (arg: T) => Promise<R>) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: CHAVE_LISTA }),
    gcTime: 0,
  })
}

export const useCriarChave = () => useMutacaoDeChave(criarChave)
export const useGirarChave = () => useMutacaoDeChave(girarChave)
export const useRevogarChave = () => useMutacaoDeChave(revogarChave)

export function useIps() {
  return useQuery({ queryKey: CHAVE_IPS, queryFn: buscarIps })
}

export function useSalvarIps() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: salvarIps,
    onSuccess: (ips) => queryClient.setQueryData(CHAVE_IPS, ips),
  })
}
