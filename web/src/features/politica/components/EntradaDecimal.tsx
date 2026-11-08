import { type ComponentProps, useState } from 'react'
import { Input } from '@/shared/components/ui/input'
import { escreverDecimal, lerDecimal } from '@/shared/utils/formato'

interface EntradaDecimalProps extends Omit<ComponentProps<'input'>, 'value' | 'onChange'> {
  /** Inteiro escalado (centavos, bps, ×10). NaN enquanto o texto não for um número. */
  valor: number
  casas: number
  aoMudar: (valor: number) => void
}

/**
 * Campo de número em pt-BR ("1.234,56") que devolve o inteiro escalado. Guarda o texto digitado
 * para não reescrever o que a pessoa está digitando ("2," no meio de "2,90").
 */
export function EntradaDecimal({ valor, casas, aoMudar, ...props }: EntradaDecimalProps) {
  const [texto, setTexto] = useState(() => escreverDecimal(valor, casas))
  const [visto, setVisto] = useState(valor)

  // Valor trocado por fora (reverter, recarregar): o texto acompanha.
  if (!Object.is(valor, visto)) {
    setVisto(valor)
    if (!Object.is(lerDecimal(texto, casas), valor)) setTexto(escreverDecimal(valor, casas))
  }

  return (
    <Input
      {...props}
      inputMode={casas > 0 ? 'decimal' : 'numeric'}
      autoComplete="off"
      value={texto}
      aria-invalid={props['aria-invalid'] || Number.isNaN(valor)}
      onChange={(evento) => {
        const novo = lerDecimal(evento.target.value, casas)
        setTexto(evento.target.value)
        setVisto(novo)
        aoMudar(novo)
      }}
    />
  )
}
