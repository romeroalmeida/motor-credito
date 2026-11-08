// Dinheiro chega em centavos (inteiro) e taxa em pontos-base (290 = 2,90% a.m.): a conversão para
// texto acontece só aqui, na borda da tela, e a volta (entrada do usuário → inteiro) também.

const LOCALE = 'pt-BR'

const MOEDA = new Intl.NumberFormat(LOCALE, { style: 'currency', currency: 'BRL' })
const DATA_HORA = new Intl.DateTimeFormat(LOCALE, { dateStyle: 'short', timeStyle: 'short' })
const DATA = new Intl.DateTimeFormat(LOCALE, { dateStyle: 'short' })
const INTEIRO = new Intl.NumberFormat(LOCALE)

function decimal(casas: number) {
  return new Intl.NumberFormat(LOCALE, { minimumFractionDigits: casas, maximumFractionDigits: casas })
}

/** 123456 → "R$ 1.234,56" */
export function formatarCentavos(centavos: number): string {
  return MOEDA.format(centavos / 100)
}

/** 290 → "2,90% a.m." */
export function formatarTaxaBps(bps: number): string {
  return `${decimal(2).format(bps / 100)}% a.m.`
}

/** 6500 → "65,0%" (taxa de aprovação e afins, também em pontos-base). */
export function formatarPercentualBps(bps: number): string {
  return `${decimal(1).format(bps / 100)}%`
}

/** 30 → "3,0×" (múltiplo da renda guardado ×10). */
export function formatarMultiploX10(x10: number): string {
  return `${decimal(1).format(x10 / 10)}×`
}

/** 12345 → "12.345" */
export function formatarInteiro(valor: number): string {
  return INTEIRO.format(valor)
}

export function formatarDataHora(iso: string): string {
  return DATA_HORA.format(new Date(iso))
}

export function formatarData(iso: string): string {
  return DATA.format(new Date(iso))
}

/**
 * Lê um decimal digitado em pt-BR e devolve um inteiro escalado por 10^casas, ou NaN.
 * "1.234,56" (casas 2) → 123456 · "2,9" (casas 2) → 290 · "3" (casas 1) → 30.
 * Ponto só como separador de milhar (em grupos de 3); vírgula é o decimal.
 */
export function lerDecimal(texto: string, casas: number): number {
  const limpo = texto.replace(/^\s*R\$\s*/, '').trim()
  const fracao = casas > 0 ? `(,\\d{1,${casas}})?` : ''
  const formato = new RegExp(`^(\\d{1,3}(\\.\\d{3})+|\\d+)${fracao}$`)
  if (!formato.test(limpo)) return Number.NaN
  const [inteira = '', decimais = ''] = limpo.replace(/\./g, '').split(',')
  return Number(inteira + decimais.padEnd(casas, '0'))
}

/** Inverso de lerDecimal para preencher campo de edição: 123456 (casas 2) → "1234,56". */
export function escreverDecimal(valor: number, casas: number): string {
  if (!Number.isFinite(valor)) return ''
  const fator = 10 ** casas
  const inteira = Math.trunc(valor / fator)
  if (casas === 0) return String(inteira)
  const resto = String(Math.abs(valor % fator)).padStart(casas, '0')
  return `${inteira},${resto}`
}
