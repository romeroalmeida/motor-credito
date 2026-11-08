import { Campo } from '@/shared/components'
import { Textarea } from '@/shared/components/ui/textarea'

interface EntradaCelProps {
  valor: string
  comErro: boolean
  aoMudar: (cel: string) => void
}

export function EntradaCel({ valor, comErro, aoMudar }: EntradaCelProps) {
  return (
    <Campo
      rotulo="Expressão CEL"
      ajuda="Campos: score, bureau.restricoes, bureau.protestos, bureau.divida_total, renda, idade, pedido.valor, pedido.prazo, fraude.risco, fraude.sinais. Dinheiro em centavos. Renda e idade podem faltar: use has(renda) antes."
    >
      {(props) => (
        <Textarea
          {...props}
          aria-invalid={comErro}
          className="min-h-20 font-mono text-sm md:text-sm"
          spellCheck={false}
          autoCapitalize="off"
          value={valor}
          onChange={(e) => aoMudar(e.target.value)}
        />
      )}
    </Campo>
  )
}
