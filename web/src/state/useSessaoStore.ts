import { create } from 'zustand'
import type { Schemas } from '@/shared/types'

type Eu = Schemas['Eu']

interface SessaoState {
  eu: Eu | null
  definir: (eu: Eu) => void
  limpar: () => void
}

// Só o retrato de /painel/auth/eu; a sessão em si vive no cookie httpOnly.
export const useSessaoStore = create<SessaoState>((set) => ({
  eu: null,
  definir: (eu) => set({ eu }),
  limpar: () => set({ eu: null }),
}))
