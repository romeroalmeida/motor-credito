import axios from 'axios'
import type { Schemas } from '@/shared/types'

type CodigoErro = Schemas['Erro']['erro']

export interface ProblemaRegra {
  /** Caminho no JSON da política: "cortes[0]", "faixas[2].condicoes[1]", "faixas[0].taxa_bps". */
  caminho: string
  motivo: string
}

export interface ErroApi {
  status: number | null
  codigo: CodigoErro | null
  mensagem: string
  /** Regras recusadas (422) e campos inválidos (400), no mesmo formato. */
  problemas: ProblemaRegra[]
  esperarSegundos: number | null
}

const MENSAGENS: Partial<Record<CodigoErro, string>> = {
  corpo_invalido: 'Alguns dados não foram aceitos. Confira os campos marcados.',
  nao_autorizado: 'Sua sessão terminou. Entre de novo.',
  csrf_invalido: 'A página ficou desatualizada. Recarregue e tente de novo.',
  proibido: 'Seu papel não permite esta ação.',
  nao_encontrado: 'Não encontramos o que você pediu.',
  conflito: 'Isso já existe.',
  limite_excedido: 'Muitas tentativas seguidas. Aguarde um pouco e tente de novo.',
  politica_invalida: 'A política tem regras que não compilam. Veja as marcadas abaixo.',
  erro_interno: 'O servidor falhou. Tente de novo em instantes.',
}

const SEM_CONEXAO = 'Não conseguimos falar com o servidor. Confira a conexão e tente de novo.'

function lerProblemas(corpo: Partial<Schemas['Erro']>): ProblemaRegra[] {
  const detalhes = (corpo.detalhes ?? []).map(({ regra, motivo }) => ({ caminho: regra, motivo }))
  const campos = Object.entries(corpo.campos ?? {}).map(([caminho, motivo]) => ({ caminho, motivo }))
  return [...detalhes, ...campos]
}

/** Converte qualquer erro de chamada em algo que a tela consegue mostrar, sem vazar detalhes técnicos. */
export function lerErroApi(erro: unknown): ErroApi {
  if (!axios.isAxiosError(erro) || !erro.response) {
    return { status: null, codigo: null, mensagem: SEM_CONEXAO, problemas: [], esperarSegundos: null }
  }
  const { status, data, headers } = erro.response
  const corpo: Partial<Schemas['Erro']> = typeof data === 'object' && data !== null ? data : {}
  const codigo = corpo.erro ?? null
  const esperar = Number(headers['retry-after'])
  return {
    status,
    codigo,
    mensagem: corpo.mensagem ?? (codigo && MENSAGENS[codigo]) ?? MENSAGENS.erro_interno ?? SEM_CONEXAO,
    problemas: lerProblemas(corpo),
    esperarSegundos: Number.isFinite(esperar) && esperar > 0 ? esperar : null,
  }
}

export function statusDoErro(erro: unknown): number | null {
  return axios.isAxiosError(erro) ? (erro.response?.status ?? null) : null
}
