import type { ReactNode } from 'react'

interface CabecalhoPaginaProps {
  titulo: string
  descricao?: ReactNode
  acoes?: ReactNode
  /** Link de volta (ex.: detalhe → lista), renderizado acima do título. */
  voltar?: ReactNode
}

export const ID_TITULO_PAGINA = 'titulo-pagina'

export function CabecalhoPagina({ titulo, descricao, acoes, voltar }: CabecalhoPaginaProps) {
  return (
    <header className="flex flex-wrap items-end justify-between gap-4 pt-2 pb-2 md:pt-4">
      <div className="min-w-0 space-y-1.5">
        {voltar}
        <h1 id={ID_TITULO_PAGINA} className="text-2xl font-semibold tracking-tight text-balance md:text-3xl">
          {titulo}
        </h1>
        {descricao && <p className="max-w-prose text-sm text-muted-foreground">{descricao}</p>}
      </div>
      {acoes && <div className="flex flex-wrap items-center gap-2">{acoes}</div>}
    </header>
  )
}
