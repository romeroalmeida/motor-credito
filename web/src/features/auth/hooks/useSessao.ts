import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useSessaoStore } from '@/state/useSessaoStore'
import { buscarEu, cadastrar, confirmarDoisFatores, entrar, type Eu, sair } from '../services/authService'

export const CHAVE_EU = ['sessao', 'eu'] as const

/** GET /painel/auth/eu: a guarda de rota decide pelo resultado (401 → Entrar). */
export function useEu() {
  const definir = useSessaoStore((estado) => estado.definir)
  return useQuery({
    queryKey: CHAVE_EU,
    queryFn: async () => {
      const eu = await buscarEu()
      definir(eu)
      return eu
    },
    retry: false,
  })
}

export function useEntrar() {
  return useMutation({ mutationFn: entrar })
}

export function useCadastrar() {
  return useMutation({ mutationFn: cadastrar })
}

/** QR do otpauth gerado no navegador (o segredo não sai para serviço nenhum); a lib vem num chunk à parte. */
export function useQrCode(otpauth: string | undefined) {
  return useQuery({
    queryKey: ['cadastro', 'qr', otpauth],
    queryFn: async () => {
      const { toDataURL } = await import('qrcode')
      return toDataURL(otpauth ?? '', { margin: 1, width: 208 })
    },
    enabled: Boolean(otpauth),
    staleTime: Infinity,
  })
}

export function useConfirmarDoisFatores() {
  const queryClient = useQueryClient()
  const definir = useSessaoStore((estado) => estado.definir)
  return useMutation({
    mutationFn: confirmarDoisFatores,
    onSuccess: (eu: Eu) => {
      definir(eu)
      queryClient.setQueryData(CHAVE_EU, eu)
    },
  })
}

export function useSair() {
  const queryClient = useQueryClient()
  const limpar = useSessaoStore((estado) => estado.limpar)
  return useMutation({
    mutationFn: sair,
    // Mesmo se a chamada falhar, a tela esquece tudo o que sabia da sessão.
    onSettled: () => {
      limpar()
      queryClient.clear()
    },
  })
}
