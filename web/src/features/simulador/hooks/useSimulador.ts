import { useMutation, useQuery } from '@tanstack/react-query'
import { listarChavesAtivas, simular } from '../services/simuladorService'

export function useChavesAtivas() {
  return useQuery({ queryKey: ['simulador', 'chaves-ativas'], queryFn: listarChavesAtivas })
}

export function useSimular() {
  return useMutation({ mutationFn: simular })
}
