import { Lock } from 'lucide-react'
import type { ReactNode } from 'react'
import { Cartao, Vazio } from '@/shared/components'
import { type Area, podeVer } from '@/shared/utils/permissoes'
import { usePapel } from '@/state/useSessaoStore'

/** O servidor responde 403 de todo jeito; aqui a tela explica em vez de mostrar um erro genérico. */
export function ExigirPapel({ area, children }: { area: Area; children: ReactNode }) {
  const papel = usePapel()
  if (podeVer(papel, area)) return children
  return (
    <Cartao className="mt-4">
      <Vazio icone={Lock} titulo="Seu papel não dá acesso a esta área">
        Peça a um administrador da sua equipe se precisar ver estas informações.
      </Vazio>
    </Cartao>
  )
}
