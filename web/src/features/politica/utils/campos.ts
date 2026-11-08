import type { Schemas } from '@/shared/types'

// Campos do contexto CEL (desenho §5) com o jeito de editar cada um na tela.

export type Condicao = Schemas['Condicao']
export type NomeCampo = Condicao['campo']
export type Operador = Condicao['operador']
type Sinal = Schemas['Antifraude']['sinais'][number]

export type TipoCampo = 'inteiro' | 'dinheiro' | 'sinal'

interface DefinicaoCampo {
  rotulo: string
  tipo: TipoCampo
  /** Campo que pode faltar no contexto: no CEL precisa de has(). */
  opcional?: boolean
}

export const CAMPOS: Record<NomeCampo, DefinicaoCampo> = {
  score: { rotulo: 'Score (0–1000)', tipo: 'inteiro' },
  'bureau.restricoes': { rotulo: 'Restrições no bureau', tipo: 'inteiro' },
  'bureau.protestos': { rotulo: 'Protestos', tipo: 'inteiro' },
  'bureau.divida_total': { rotulo: 'Dívida total', tipo: 'dinheiro' },
  renda: { rotulo: 'Renda mensal', tipo: 'dinheiro', opcional: true },
  idade: { rotulo: 'Idade', tipo: 'inteiro', opcional: true },
  'pedido.valor': { rotulo: 'Valor pedido', tipo: 'dinheiro' },
  'pedido.prazo': { rotulo: 'Prazo pedido (meses)', tipo: 'inteiro' },
  'fraude.risco': { rotulo: 'Risco de fraude (0–100)', tipo: 'inteiro' },
  'fraude.sinais': { rotulo: 'Sinais de fraude', tipo: 'sinal' },
}

export const NOMES_CAMPOS = Object.keys(CAMPOS) as NomeCampo[]

export const OPERADORES_NUMERICOS: Operador[] = ['>=', '>', '<=', '<', '==', '!=']

export const ROTULO_OPERADOR: Record<Operador, string> = {
  '>=': 'maior ou igual a',
  '>': 'maior que',
  '<=': 'menor ou igual a',
  '<': 'menor que',
  '==': 'igual a',
  '!=': 'diferente de',
  in: 'contém',
}

export const SINAIS: Sinal[] = [
  'cpf_irregular',
  'cnpj_inativo',
  'velocidade_alta',
  'nome_divergente',
  'nascimento_divergente',
  'email_descartavel',
  'ip_estrangeiro',
  'ip_datacenter',
  'dispositivo_multiplos_cpfs',
  'cnpj_recente',
]

export function operadoresDo(campo: NomeCampo): Operador[] {
  // "fraude.sinais" é lista: a única pergunta que faz sentido é se contém um sinal.
  return CAMPOS[campo].tipo === 'sinal' ? ['in'] : OPERADORES_NUMERICOS
}

/** Condição nova (ou ao trocar de campo) com operador e valor coerentes com o tipo. */
export function condicaoPadrao(campo: NomeCampo): Condicao {
  return CAMPOS[campo].tipo === 'sinal'
    ? { campo, operador: 'in', valor: 'email_descartavel' }
    : { campo, operador: '>=', valor: 0 }
}

function literal(valor: Condicao['valor']): string {
  if (Array.isArray(valor)) return `[${valor.map(literal).join(', ')}]`
  return typeof valor === 'string' ? JSON.stringify(valor) : String(valor)
}

/** "in" em fraude.sinais quer dizer "contém": com lista, basta um dos sinais (igual ao servidor). */
function sinaisParaCel(valor: Condicao['valor']): string {
  const sinais = Array.isArray(valor) ? valor : [valor]
  const partes = sinais.map((sinal) => `${literal(sinal)} in fraude.sinais`)
  return partes.length === 1 ? partes.join('') : `(${partes.join(' || ')})`
}

/** Uma condição do editor visual em CEL. Campo opcional ganha has(), como o servidor exige. */
export function condicaoParaCel({ campo, operador, valor }: Condicao): string {
  if (CAMPOS[campo].tipo === 'sinal') return sinaisParaCel(valor)
  const comparacao = `${campo} ${operador} ${literal(valor)}`
  return CAMPOS[campo].opcional ? `has(${campo}) && ${comparacao}` : comparacao
}

/** Condições ligadas por E. Sem condição, a regra sempre bate. */
export function condicoesParaCel(condicoes: Condicao[]): string {
  return condicoes.length === 0 ? 'true' : condicoes.map(condicaoParaCel).join(' && ')
}
