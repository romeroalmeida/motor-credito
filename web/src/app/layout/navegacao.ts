import {
  Gauge,
  KeyRound,
  ListChecks,
  type LucideIcon,
  ScrollText,
  SlidersHorizontal,
  Users,
} from 'lucide-react'
import type { Area } from '@/shared/utils/permissoes'

export interface ItemNavegacao {
  rota: string
  rotulo: string
  icone: LucideIcon
  area: Area
}

export const ITENS_NAVEGACAO: readonly ItemNavegacao[] = [
  { rota: '/visao-geral', rotulo: 'Visão geral', icone: Gauge, area: 'visao-geral' },
  { rota: '/consultas', rotulo: 'Consultas', icone: ListChecks, area: 'consultas' },
  { rota: '/politica', rotulo: 'Política', icone: SlidersHorizontal, area: 'politica' },
  { rota: '/chaves', rotulo: 'Chaves de API', icone: KeyRound, area: 'chaves' },
  { rota: '/equipe', rotulo: 'Equipe', icone: Users, area: 'equipe' },
  { rota: '/auditoria', rotulo: 'Auditoria', icone: ScrollText, area: 'auditoria' },
]
