import { Navigate } from 'react-router-dom'
import { type Area, podeVer } from '@/shared/utils/permissoes'
import { usePapel } from '@/state/useSessaoStore'

const AREAS_INICIAIS: readonly { area: Area; rota: string }[] = [
  { area: 'visao-geral', rota: '/visao-geral' },
  { area: 'auditoria', rota: '/auditoria' },
]

/** A página inicial depende do papel: o auditor só tem a auditoria. */
export function Inicio() {
  const papel = usePapel()
  const destino = AREAS_INICIAIS.find(({ area }) => podeVer(papel, area))?.rota ?? '/visao-geral'
  return <Navigate to={destino} replace />
}
