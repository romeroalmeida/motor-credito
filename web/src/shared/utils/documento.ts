// Espelho de internal/documento (Go): CPF e CNPJ alfanumérico (Receita, 2026).
// CNPJ: 12 primeiras posições [0-9A-Z], 2 últimas numéricas; valor do caractere = ASCII−48 (não é base 36).

const TAMANHO_CPF = 11
const TAMANHO_CNPJ = 14
const MASCARA_CPF = '###.###.###-##'
const MASCARA_CNPJ = '##.###.###/####-##'

/** Tira a máscara (. - / espaço) e põe em maiúscula. Outros caracteres são descartados. */
export function normalizarDocumento(entrada: string): string {
  return entrada.toUpperCase().replace(/[^0-9A-Z]/g, '')
}

/** Valor do caractere no cálculo do DV: '0'..'9' → 0..9, 'A' → 17, 'Z' → 42. */
function valorDoCaractere(caractere: string): number {
  return caractere.charCodeAt(0) - 48
}

function pesoCpf(posicaoDv: number, j: number): number {
  return posicaoDv + 1 - j
}

// CNPJ: ciclo 2..9 da direita para a esquerda.
function pesoCnpj(posicaoDv: number, j: number): number {
  return ((posicaoDv - j - 1) % 8) + 2
}

function dvConfere(doc: string, base: number, peso: (posicaoDv: number, j: number) => number): boolean {
  for (let posicaoDv = base; posicaoDv < base + 2; posicaoDv++) {
    let soma = 0
    for (let j = 0; j < posicaoDv; j++) soma += valorDoCaractere(doc.charAt(j)) * peso(posicaoDv, j)
    const resto = soma % 11
    const dv = resto < 2 ? 0 : 11 - resto
    if (valorDoCaractere(doc.charAt(posicaoDv)) !== dv) return false
  }
  return true
}

function todosIguais(doc: string): boolean {
  return [...doc].every((c) => c === doc.charAt(0))
}

/** CPF (11 dígitos) ou CNPJ alfanumérico (14) com dígitos verificadores corretos. Aceita com ou sem máscara. */
export function documentoValido(entrada: string): boolean {
  if (/[^0-9A-Za-z.\-/\s]/.test(entrada)) return false
  const doc = normalizarDocumento(entrada)
  if (doc === '' || todosIguais(doc)) return false
  if (doc.length === TAMANHO_CPF) return /^\d+$/.test(doc) && dvConfere(doc, 9, pesoCpf)
  if (doc.length === TAMANHO_CNPJ) return /^\d{2}$/.test(doc.slice(12)) && dvConfere(doc, 12, pesoCnpj)
  return false
}

/** Máscara progressiva para campo de digitação: CPF enquanto couber em 11 dígitos, senão CNPJ. */
export function mascararDocumento(entrada: string): string {
  const doc = normalizarDocumento(entrada).slice(0, TAMANHO_CNPJ)
  const ehCpf = doc.length <= TAMANHO_CPF && /^\d*$/.test(doc)
  const mascara = ehCpf ? MASCARA_CPF : MASCARA_CNPJ
  let saida = ''
  let i = 0
  for (const simbolo of mascara) {
    if (i >= doc.length) break
    saida += simbolo === '#' ? doc.charAt(i++) : simbolo
  }
  return saida
}
