import type { ErroApi } from '@/shared/utils/erroApi'

export function mensagemLogin(erro: ErroApi, passo: 'senha' | 'codigo'): string {
  if (erro.status === 429) {
    const espera = erro.esperarSegundos
      ? ` Tente de novo em ${erro.esperarSegundos} s.`
      : ' Aguarde um pouco.'
    return `Muitas tentativas seguidas.${espera}`
  }
  if (erro.status === 401) {
    return passo === 'senha'
      ? 'E-mail ou senha não conferem.'
      : 'Código não confere ou expirou. Confira o app autenticador ou volte e entre de novo.'
  }
  return erro.mensagem
}
