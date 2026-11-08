import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  buscarDiferenca,
  buscarEstado,
  listarVersoes,
  publicarRascunho,
  reverterVersao,
  salvarRascunho,
  testarPolitica,
} from '../services/politicaService'

const CHAVE = {
  estado: ['politica', 'estado'],
  versoes: ['politica', 'versoes'],
  diferenca: (de: number, para: string) => ['politica', 'diferenca', de, para],
} as const

export function useEstadoPolitica() {
  return useQuery({ queryKey: CHAVE.estado, queryFn: buscarEstado })
}

export function useVersoes() {
  return useQuery({ queryKey: CHAVE.versoes, queryFn: listarVersoes })
}

export function useDiferenca(de: number | null, para: string) {
  return useQuery({
    queryKey: CHAVE.diferenca(de ?? 0, para),
    queryFn: () => buscarDiferenca(de ?? 0, para),
    enabled: de !== null,
  })
}

export function useSalvarRascunho() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: salvarRascunho,
    onSuccess: (estado) => {
      queryClient.setQueryData(CHAVE.estado, estado)
      void queryClient.invalidateQueries({ queryKey: ['politica', 'diferenca'] })
    },
  })
}

export function useTestarPolitica() {
  return useMutation({ mutationFn: testarPolitica })
}

function useAoPublicar() {
  const queryClient = useQueryClient()
  return () => queryClient.invalidateQueries({ queryKey: ['politica'] })
}

export function usePublicar() {
  const aoPublicar = useAoPublicar()
  return useMutation({ mutationFn: publicarRascunho, onSuccess: aoPublicar })
}

export function useReverter() {
  const aoPublicar = useAoPublicar()
  return useMutation({ mutationFn: reverterVersao, onSuccess: aoPublicar })
}
