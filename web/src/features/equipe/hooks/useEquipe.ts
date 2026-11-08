import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { buscarEquipe, convidar } from '../services/equipeService'

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
