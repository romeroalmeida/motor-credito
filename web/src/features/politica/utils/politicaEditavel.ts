import type { Schemas } from '@/shared/types'
import type { ProblemaRegra } from '@/shared/utils/erroApi'
import { CAMPOS, type Condicao } from './campos'

// O editor trabalha numa cópia da política com uma chave estável por item (para o React não
// trocar campos de lugar ao remover ou reordenar). As chaves somem antes de mandar ao servidor.

export type Politica = Schemas['Politica']

export interface CondicaoEditavel extends Condicao {
  chave: string
}
export interface CorteEditavel extends Omit<Schemas['Corte'], 'condicoes'> {
  chave: string
  condicoes: CondicaoEditavel[]
}
export interface FaixaEditavel extends Omit<Schemas['Faixa'], 'condicoes'> {
  chave: string
  condicoes: CondicaoEditavel[]
}
export interface PoliticaEditavel {
  cortes: CorteEditavel[]
  faixas: FaixaEditavel[]
  contraproposta: Schemas['Contraproposta']
  expressao_avancada: boolean
}

let sequencia = 0
export function novaChave(): string {
  sequencia += 1
  return `k${sequencia}`
}

function comChaves(condicoes: Condicao[] = []): CondicaoEditavel[] {
  return condicoes.map((condicao) => ({ ...condicao, chave: novaChave() }))
}

function semChaves(condicoes: CondicaoEditavel[]): Condicao[] {
  return condicoes.map(({ campo, operador, valor }) => ({ campo, operador, valor }))
}

export function paraEditavel(politica: Politica): PoliticaEditavel {
  return {
    cortes: politica.cortes.map((corte) => ({
      ...corte,
      chave: novaChave(),
      condicoes: comChaves(corte.condicoes),
    })),
    faixas: politica.faixas.map((faixa) => ({
      ...faixa,
      chave: novaChave(),
      condicoes: comChaves(faixa.condicoes),
    })),
    contraproposta: { ...politica.contraproposta },
    expressao_avancada: politica.expressao_avancada,
  }
}

export function paraPolitica(editavel: PoliticaEditavel): Politica {
  return {
    cortes: editavel.cortes.map(({ codigo, texto, condicoes, cel }) => ({
      codigo,
      texto,
      condicoes: semChaves(condicoes),
      cel,
    })),
    faixas: editavel.faixas.map(
      ({ rotulo, condicoes, cel, nega, taxa_bps, limite_fixo, multiplo_renda_x10 }) => ({
        rotulo,
        condicoes: semChaves(condicoes),
        cel,
        nega,
        taxa_bps,
        limite_fixo,
        multiplo_renda_x10,
      }),
    ),
    contraproposta: { ...editavel.contraproposta },
    expressao_avancada: editavel.expressao_avancada,
  }
}

// Mesmos limites do Compilar (internal/politica): até 12 prazos, cada um de 1 a 420 meses, sem repetir.
export const PRAZO_MAXIMO_MESES = 420
export const MAX_PRAZOS_OFERTADOS = 12
/** O que o motor simula quando a política não lista prazos (cortado no prazo máximo da oferta). */
export const PRAZOS_PADRAO = [12, 24, 36]

/** Erro ao incluir o prazo digitado, ou null quando cabe na lista. prazoMaximo 0 = sem teto da oferta. */
export function erroNovoPrazo(prazos: number[], prazo: number, prazoMaximo: number): string | null {
  if (!Number.isInteger(prazo) || prazo < 1 || prazo > PRAZO_MAXIMO_MESES)
    return `Prazo de 1 a ${PRAZO_MAXIMO_MESES} meses.`
  if (prazoMaximo > 0 && prazo > prazoMaximo) return `Acima do prazo máximo da oferta (${prazoMaximo} meses).`
  if (prazos.includes(prazo)) return 'Esse prazo já está na lista.'
  if (prazos.length >= MAX_PRAZOS_OFERTADOS) return `No máximo ${MAX_PRAZOS_OFERTADOS} prazos.`
  return null
}

export function novoCorte(): CorteEditavel {
  return { chave: novaChave(), codigo: '', texto: '', condicoes: [], cel: '' }
}

export function novaFaixa(rotulo: string): FaixaEditavel {
  return {
    chave: novaChave(),
    rotulo,
    condicoes: [],
    cel: '',
    nega: false,
    taxa_bps: 0,
    limite_fixo: 0,
    multiplo_renda_x10: 0,
  }
}

const invalido = (n: number) => !Number.isInteger(n) || n < 0

function problemasCondicoes(base: string, condicoes: CondicaoEditavel[]): ProblemaRegra[] {
  return condicoes.flatMap((condicao, j) =>
    CAMPOS[condicao.campo].tipo !== 'sinal' && invalido(Number(condicao.valor))
      ? [{ caminho: `${base}.condicoes[${j}]`, motivo: `Condição ${j + 1}: valor inválido.` }]
      : [],
  )
}

/**
 * O que dá para conferir antes de mandar ao servidor: campos vazios e números ilegíveis.
 * Tipos e compilação do CEL ficam com o servidor (422, ver lerErroApi).
 */
export function validarLocal(p: PoliticaEditavel): ProblemaRegra[] {
  const problemas: ProblemaRegra[] = []
  const avancado = p.expressao_avancada

  p.cortes.forEach((corte, i) => {
    const base = `cortes[${i}]`
    if (!corte.codigo.trim()) problemas.push({ caminho: `${base}.codigo`, motivo: 'Dê um código ao corte.' })
    if (!corte.texto.trim())
      problemas.push({ caminho: `${base}.texto`, motivo: 'Escreva o motivo do corte.' })
    if (avancado && !corte.cel?.trim())
      problemas.push({ caminho: `${base}.cel`, motivo: 'Escreva a expressão CEL.' })
    // Corte sem condição negaria todo mundo.
    if (!avancado && corte.condicoes.length === 0) {
      problemas.push({
        caminho: base,
        motivo: 'Inclua ao menos uma condição; sem ela o corte negaria todos.',
      })
    }
    if (!avancado) problemas.push(...problemasCondicoes(base, corte.condicoes))
  })

  if (p.faixas.length === 0)
    problemas.push({ caminho: 'faixas', motivo: 'A política precisa de ao menos uma faixa.' })
  p.faixas.forEach((faixa, i) => {
    const base = `faixas[${i}]`
    if (!faixa.rotulo.trim()) problemas.push({ caminho: `${base}.rotulo`, motivo: 'Dê um rótulo à faixa.' })
    if (avancado && !faixa.cel?.trim()) {
      problemas.push({
        caminho: `${base}.cel`,
        motivo: 'Escreva a expressão CEL (use true para pegar todos).',
      })
    }
    if (!avancado) problemas.push(...problemasCondicoes(base, faixa.condicoes))
    if (faixa.nega) return
    if (invalido(faixa.taxa_bps))
      problemas.push({ caminho: `${base}.taxa_bps`, motivo: 'Taxa inválida. Ex.: 2,90' })
    if (invalido(faixa.limite_fixo)) {
      problemas.push({ caminho: `${base}.limite_fixo`, motivo: 'Limite inválido. Ex.: 5.000,00' })
    }
    if (invalido(faixa.multiplo_renda_x10)) {
      problemas.push({ caminho: `${base}.multiplo_renda_x10`, motivo: 'Múltiplo inválido. Ex.: 3,0' })
    }
  })

  const { prazo_maximo, acrescimo_taxa_bps } = p.contraproposta
  // Versão antiga pode vir sem o campo (ou null): vale o padrão do motor.
  const prazos_ofertados = p.contraproposta.prazos_ofertados ?? []
  if (invalido(prazo_maximo))
    problemas.push({ caminho: 'contraproposta.prazo_maximo', motivo: 'Prazo inválido.' })
  // O prazo máximo pode ter baixado depois de os prazos entrarem na lista.
  const acima = prazos_ofertados.filter((prazo) => prazo_maximo > 0 && prazo > prazo_maximo)
  if (acima.length > 0) {
    problemas.push({
      caminho: 'contraproposta.prazos_ofertados',
      motivo: `Prazos acima do prazo máximo (${prazo_maximo} meses): ${acima.join(', ')}.`,
    })
  }
  if (invalido(acrescimo_taxa_bps)) {
    problemas.push({ caminho: 'contraproposta.acrescimo_taxa_bps', motivo: 'Acréscimo inválido.' })
  }
  return problemas
}

/** Problemas que pertencem a um caminho ou a qualquer coisa dentro dele ("faixas[1]" pega "faixas[1].cel"). */
export function problemasEm(problemas: ProblemaRegra[], prefixo: string): ProblemaRegra[] {
  return problemas.filter(
    ({ caminho }) =>
      caminho === prefixo || caminho.startsWith(`${prefixo}.`) || caminho.startsWith(`${prefixo}[`),
  )
}

export function temProblema(problemas: ProblemaRegra[], caminho: string): boolean {
  return problemasEm(problemas, caminho).length > 0
}

export type Aba = 'cortes' | 'faixas' | 'contraproposta' | 'historico'

/** Aba onde o primeiro problema aparece, para levar a pessoa direto até ele. */
export function abaDoProblema(problemas: ProblemaRegra[]): Aba | null {
  const primeiro = problemas[0]?.caminho ?? ''
  if (primeiro.startsWith('cortes')) return 'cortes'
  if (primeiro.startsWith('faixas')) return 'faixas'
  if (primeiro.startsWith('contraproposta')) return 'contraproposta'
  return null
}

/** Move um item da lista para outra posição, sem mutar a original. */
export function mover<T>(lista: T[], de: number, para: number): T[] {
  const copia = [...lista]
  const [item] = copia.splice(de, 1)
  if (item !== undefined) copia.splice(para, 0, item)
  return copia
}
