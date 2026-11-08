import { Gauge, KeyRound, ListChecks, type LucideIcon, SlidersHorizontal, Users } from 'lucide-react'

export interface ItemNavegacao {
  rota: string
  rotulo: string
  icone: LucideIcon
}

export const ITENS_NAVEGACAO: readonly ItemNavegacao[] = [
  { rota: '/visao-geral', rotulo: 'Visão geral', icone: Gauge },
  { rota: '/consultas', rotulo: 'Consultas', icone: ListChecks },
  { rota: '/politica', rotulo: 'Política', icone: SlidersHorizontal },
  { rota: '/chaves', rotulo: 'Chaves de API', icone: KeyRound },
  { rota: '/equipe', rotulo: 'Equipe', icone: Users },
]
