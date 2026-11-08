import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { buscarEquipe, convidar, removerMembro, trocarPapel } from '../services/equipeService'

const CHAVE = ['equipe'] as const

export function useEquipe() {
  return useQuery({ queryKey: CHAVE, queryFn: buscarEquipe })
}

export function useConvidar() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: convidar,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: CHAVE }),
    // O link do convite só existe nesta resposta: não fica guardado depois de fechar.
    gcTime: 0,
  })
}

export function useTrocarPapel() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: trocarPapel,
    onSettled: () => queryClient.invalidateQueries({ queryKey: CHAVE }),
  })
}

export function useRemoverMembro() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: removerMembro,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: CHAVE }),
  })
}
