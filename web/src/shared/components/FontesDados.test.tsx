import { render, screen, within } from '@testing-library/react'
import type { Schemas } from '@/shared/types'
import { FontesDados } from './ResultadoDecisao'

type Fontes = Schemas['Fontes']

const CASOS_CADASTRO: [Fontes['cadastro'], string, string][] = [
  ['serpro', 'Real · Serpro', 'bg-sinal-aprovado'],
  ['serpro-trial', 'Real · Serpro (ambiente de testes)', 'bg-sinal-aprovado'],
  ['brasilapi', 'Real · BrasilAPI', 'bg-sinal-aprovado'],
  ['simulado', 'Simulado', 'bg-sinal-neutro'],
  ['nao_verificado', 'Não verificado', 'bg-sinal-alerta'],
]

const linha = (rotulo: string) => screen.getByText(rotulo).parentElement as HTMLElement

describe('FontesDados', () => {
  it.each(CASOS_CADASTRO)(
    'cadastro %s mostra o selo "%s" com texto (a cor %s só reforça)',
    (cadastro, texto, cor) => {
      render(<FontesDados fontes={{ cadastro, score: 'simulado', restricoes: 'simulado' }} />)
      const selo = within(linha('Cadastro')).getByText(texto)
      expect(selo.querySelector('[aria-hidden="true"]')).toHaveClass(cor)
    },
  )

  it('score cadastral mostra o selo, os fatores e o cadastro público do CNPJ', () => {
    render(
      <FontesDados
        fontes={{ cadastro: 'brasilapi', score: 'cadastral', restricoes: 'simulado' }}
        fatores={[
          { codigo: 'SITUACAO_ATIVA', pontos: 250 },
          { codigo: 'REGIME_MEI', pontos: 0 },
        ]}
        cadastro={{ situacao: 'regular', razao_social: 'BANCO DO BRASIL SA', abertura: '1966-08-01' }}
      />,
    )
    expect(within(linha('Score')).getByText('Cadastral (dados públicos)')).toBeInTheDocument()
    expect(within(linha('Restrições e protestos')).getByText('Simulado')).toBeInTheDocument()
    const fatores = screen.getByRole('list', { name: 'Fatores do score cadastral' })
    expect(within(fatores).getByText('SITUACAO_ATIVA').nextSibling).toHaveTextContent('+250')
    expect(within(fatores).getByText('REGIME_MEI').nextSibling).toHaveTextContent('+0')
    expect(screen.getByText('BANCO DO BRASIL SA')).toBeInTheDocument()
    expect(screen.getByText('01/08/1966')).toBeInTheDocument()
    expect(screen.getByText('Regular')).toBeInTheDocument()
  })

  it('não verificado explica que falta o nascimento; sem fatores não mostra a lista', () => {
    render(
      <FontesDados
        fontes={{ cadastro: 'nao_verificado', score: 'simulado', restricoes: 'simulado' }}
        fatores={null}
      />,
    )
    expect(screen.getByText(/exige o nascimento/)).toBeInTheDocument()
    expect(screen.queryByRole('list', { name: 'Fatores do score cadastral' })).not.toBeInTheDocument()
  })

  it('sem fontes (bureau não respondeu) diz isso', () => {
    render(<FontesDados fontes={null} />)
    expect(screen.getByText('Nenhuma fonte de dados respondeu nesta decisão.')).toBeInTheDocument()
  })
})
