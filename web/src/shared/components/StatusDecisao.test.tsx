import { render, screen } from '@testing-library/react'
import type { StatusDecisao as Status } from '@/shared/types'
import { StatusDecisao } from './StatusDecisao'

const CASOS: [Status, string, string][] = [
  ['aprovado', 'Aprovado', 'bg-sinal-aprovado'],
  ['negado', 'Negado', 'bg-sinal-negado'],
  ['contraproposta', 'Contraproposta', 'bg-sinal-contraproposta'],
  ['pre_aprovado', 'Pré-aprovado', 'bg-sinal-pre-aprovado'],
  ['documento_invalido', 'Documento inválido', 'bg-sinal-neutro'],
  ['em_analise', 'Em análise', 'bg-sinal-alerta'],
]

describe('StatusDecisao', () => {
  it.each(CASOS)('%s mostra o texto "%s" e a bolinha %s', (status, rotulo, cor) => {
    render(<StatusDecisao status={status} />)
    expect(screen.getByText(rotulo)).toBeInTheDocument()
    const bolinha = screen.getByTestId('bolinha')
    expect(bolinha).toHaveClass(cor)
    expect(bolinha).toHaveAttribute('aria-hidden', 'true')
  })
})
