/** JSON em texto ou já decodificado → indentado para leitura; texto que não é JSON volta como veio. */
export function jsonLegivel(valor: unknown): string {
  if (typeof valor !== 'string') return JSON.stringify(valor, null, 2)
  try {
    return JSON.stringify(JSON.parse(valor), null, 2)
  } catch {
    return valor
  }
}
