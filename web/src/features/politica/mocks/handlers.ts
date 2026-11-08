import { delay, type HttpHandler, http, HttpResponse } from 'msw'
import type { Schemas } from '@/shared/types'
import { type Condicao, condicoesParaCel } from '../utils/campos'
import { modeloEmprestimoPF } from './modelo'

// Handlers MSW da feature, montados em src/mocks/handlers.ts (dev e Vitest).
// Guarda rascunho e versões em memória e simula o motor só o bastante para o "testar" fazer sentido.

type Politica = Schemas['Politica']
type Versao = Schemas['PoliticaVersao']
type Mudanca = Schemas['Mudanca']
type Detalhe = { regra: string; motivo: string }

const copiar = <T>(valor: T): T => structuredClone(valor)

function versaoInicial(): Politica {
  const p = modeloEmprestimoPF()
  const faixaB = p.faixas[1]
  if (faixaB) faixaB.taxa_bps = 690
  p.contraproposta.ativa = false
  return p
}

function versaoDois(): Politica {
  const p = modeloEmprestimoPF()
  p.contraproposta.ativa = false
  return p
}

function rascunhoInicial(): Politica {
  const p = modeloEmprestimoPF()
  const faixaA = p.faixas[2]
  if (faixaA) faixaA.taxa_bps = 270
  return p
}

const versoes: Versao[] = [
  {
    versao: 3,
    publicada_por: 'Ana Ribeiro',
    publicada_em: '2026-11-03T13:20:00Z',
    revertida_de: null,
    comentario: 'Liga a contraproposta',
    conteudo: modeloEmprestimoPF(),
  },
  {
    versao: 2,
    publicada_por: 'Bruno Costa',
    publicada_em: '2026-10-21T17:05:00Z',
    revertida_de: null,
    comentario: 'Faixa B de 6,90% para 5,90%',
    conteudo: versaoDois(),
  },
  {
    versao: 1,
    publicada_por: 'Ana Ribeiro',
    publicada_em: '2026-10-02T12:00:00Z',
    revertida_de: null,
    comentario: 'Modelo inicial Empréstimo PF',
    conteudo: versaoInicial(),
  },
]

const estado = {
  rascunho: rascunhoInicial(),
  atualizadoEm: '2026-11-06T18:40:00Z',
  atualizadoPor: 'Ana Ribeiro' as string | null,
}

const publicada = () => versoes[0] ?? null

function montarEstado(): Schemas['PoliticaEstado'] {
  const atual = publicada()
  return {
    id: 'pol_01JC8ZK3Q0EMPRESTIMOPF00',
    nome: 'Empréstimo PF',
    rascunho: estado.rascunho,
    rascunho_atualizado_em: estado.atualizadoEm,
    rascunho_atualizado_por: estado.atualizadoPor,
    versao_publicada: atual?.versao ?? null,
    rascunho_difere_da_publicada: JSON.stringify(atual?.conteudo) !== JSON.stringify(estado.rascunho),
  }
}

// ------------------------------------------------------------ validação (imita o compilador CEL do servidor)

function opcionalSemHas(cel: string): string | null {
  for (const campo of ['renda', 'idade']) {
    if (new RegExp(`\\b${campo}\\b`).test(cel) && !cel.includes(`has(${campo})`))
      return `${campo} usada sem has(${campo})`
  }
  return null
}

function compilar(p: Politica): Detalhe[] {
  const detalhes: Detalhe[] = []
  if (p.faixas.length === 0) detalhes.push({ regra: 'faixas', motivo: 'ao menos uma faixa é obrigatória' })
  if (!p.expressao_avancada) return detalhes
  const regras = [
    ...p.cortes.map((r, i) => ({ regra: `cortes[${i}]`, cel: r.cel ?? '' })),
    ...p.faixas.map((r, i) => ({ regra: `faixas[${i}]`, cel: r.cel ?? '' })),
  ]
  for (const { regra, cel } of regras) {
    const motivo = cel.trim() === '' ? 'expressão vazia' : opcionalSemHas(cel)
    if (motivo) detalhes.push({ regra, motivo })
  }
  return detalhes
}

const politicaInvalida = (detalhes: Detalhe[]) =>
  HttpResponse.json({ erro: 'politica_invalida', detalhes }, { status: 422 })

// ------------------------------------------------------------ simulação do motor para "testar"

type Contexto = Record<Condicao['campo'], number | string[] | undefined>

function contextoDoDocumento(documento: string, pedido: Schemas['Pedido'], renda?: number): Contexto {
  const digitos = documento.replace(/\D/g, '')
  const soma = [...digitos].reduce((total, d, i) => total + Number(d) * (i + 7), 0)
  const score = (soma * 37) % 1001
  return {
    score,
    'bureau.restricoes': score < 250 ? 3 : 0,
    'bureau.protestos': 0,
    'bureau.divida_total': score < 400 ? 850000 : 0,
    renda,
    idade: 34,
    'pedido.valor': pedido.valor,
    'pedido.prazo': pedido.prazo,
    'fraude.risco': score % 45,
    'fraude.sinais': score % 3 === 0 ? ['email_descartavel'] : [],
  }
}

function bate(c: Condicao, ctx: Contexto): boolean {
  const atual = ctx[c.campo]
  if (atual === undefined) return false
  if (Array.isArray(atual)) return atual.includes(String(c.valor))
  const v = Number(c.valor)
  const comparacoes: Record<Condicao['operador'], boolean> = {
    '==': atual === v,
    '!=': atual !== v,
    '<': atual < v,
    '<=': atual <= v,
    '>': atual > v,
    '>=': atual >= v,
    in: false,
  }
  return comparacoes[c.operador]
}

// shortcut: em modo CEL o mock avalia as condições visuais guardadas; o motor real (agente B) avalia o CEL.
const expressao = (r: { condicoes?: Condicao[]; cel?: string }, avancado: boolean) =>
  avancado && r.cel ? r.cel : condicoesParaCel(r.condicoes ?? [])
const regraBate = (r: { condicoes?: Condicao[] }, ctx: Contexto) =>
  (r.condicoes ?? []).every((c) => bate(c, ctx))

// Parcela pela Price em float: aceitável só no mock; o servidor calcula em big.Rat.
function parcela(valor: number, prazo: number, taxaBps: number): number {
  const i = taxaBps / 10000
  return i === 0 ? Math.round(valor / prazo) : Math.round((valor * i) / (1 - (1 + i) ** -prazo))
}

function simular(p: Politica, ctx: Contexto, pedido: Schemas['Pedido']): Schemas['ResultadoTeste'] {
  const trilha: Schemas['RegraAvaliada'][] = []
  const base = { score: ctx.score as number, antifraude: antifraudeDe(ctx) }
  for (const corte of p.cortes) {
    const bateu = regraBate(corte, ctx)
    trilha.push({
      bloco: 'corte',
      rotulo: corte.codigo,
      expressao: expressao(corte, p.expressao_avancada),
      bateu,
    })
    if (bateu)
      return {
        ...base,
        status: 'negado',
        faixa_risco: null,
        oferta: null,
        motivos: [{ codigo: corte.codigo, texto: corte.texto }],
        trilha,
      }
  }
  const faixa = p.faixas.find((f) => {
    const bateu = regraBate(f, ctx)
    trilha.push({ bloco: 'faixa', rotulo: f.rotulo, expressao: expressao(f, p.expressao_avancada), bateu })
    return bateu
  })
  if (!faixa)
    return {
      ...base,
      status: 'negado',
      faixa_risco: null,
      oferta: null,
      motivos: [{ codigo: 'SEM_FAIXA', texto: 'Nenhuma faixa da política atende' }],
      trilha,
    }
  if (faixa.nega)
    return {
      ...base,
      status: 'negado',
      faixa_risco: faixa.rotulo,
      oferta: null,
      motivos: [{ codigo: 'FAIXA_NEGADA', texto: `A faixa ${faixa.rotulo} nega o pedido` }],
      trilha,
    }

  const renda = ctx.renda as number | undefined
  const limite =
    renda === undefined || faixa.multiplo_renda_x10 === 0
      ? faixa.limite_fixo
      : Math.min(faixa.limite_fixo, Math.floor((renda * faixa.multiplo_renda_x10) / 10))
  if (pedido.valor <= limite) {
    const oferta = {
      valor: pedido.valor,
      prazo: pedido.prazo,
      taxa_mensal_bps: faixa.taxa_bps,
      parcela: parcela(pedido.valor, pedido.prazo, faixa.taxa_bps),
    }
    return { ...base, status: 'aprovado', faixa_risco: faixa.rotulo, oferta, motivos: [], trilha }
  }
  const { ativa, prazo_maximo, acrescimo_taxa_bps } = p.contraproposta
  trilha.push({
    bloco: 'contraproposta',
    rotulo: ativa ? 'ativa' : 'desligada',
    expressao: `pedido.valor > ${limite}`,
    bateu: ativa,
  })
  const motivo = { codigo: 'LIMITE_FAIXA', texto: 'Valor pedido acima do limite para o score' }
  if (!ativa)
    return { ...base, status: 'negado', faixa_risco: faixa.rotulo, oferta: null, motivos: [motivo], trilha }
  const prazo = prazo_maximo > 0 ? Math.min(pedido.prazo, prazo_maximo) : pedido.prazo
  const taxa = faixa.taxa_bps + acrescimo_taxa_bps
  return {
    ...base,
    status: 'contraproposta',
    faixa_risco: faixa.rotulo,
    oferta: { valor: limite, prazo, taxa_mensal_bps: taxa, parcela: parcela(limite, prazo, taxa) },
    motivos: [motivo],
    trilha,
  }
}

function antifraudeDe(ctx: Contexto): Schemas['Antifraude'] {
  const sinais = ctx['fraude.sinais'] as Schemas['Antifraude']['sinais']
  return {
    risco: ctx['fraude.risco'] as number,
    sinais,
    checagens_executadas: ['documento', 'velocidade'],
    checagens_puladas: ['nome', 'nascimento', 'email', 'ip', 'dispositivo'],
  }
}

// ------------------------------------------------------------ diferença genérica entre dois JSON

function diferenca(antes: unknown, depois: unknown, caminho = ''): Mudanca[] {
  if (JSON.stringify(antes) === JSON.stringify(depois)) return []
  const objeto = (v: unknown) => typeof v === 'object' && v !== null
  if (!objeto(antes) || !objeto(depois) || Array.isArray(antes) !== Array.isArray(depois)) {
    return [{ caminho, tipo: 'alterado', antes, depois }]
  }
  const a = antes as Record<string, unknown>
  const b = depois as Record<string, unknown>
  const chaves = [...new Set([...Object.keys(a), ...Object.keys(b)])]
  return chaves.flatMap((chave) => {
    const filho = Array.isArray(antes) ? `${caminho}[${chave}]` : caminho ? `${caminho}.${chave}` : chave
    if (!(chave in a)) return [{ caminho: filho, tipo: 'adicionado' as const, depois: b[chave] }]
    if (!(chave in b)) return [{ caminho: filho, tipo: 'removido' as const, antes: a[chave] }]
    return diferenca(a[chave], b[chave], filho)
  })
}

const conteudoDe = (ref: string): Politica | undefined =>
  ref === 'rascunho' ? estado.rascunho : versoes.find((v) => v.versao === Number(ref))?.conteudo

function publicarComo(
  conteudo: Politica,
  comentario: string | undefined,
  revertidaDe: number | null,
): Versao {
  const nova: Versao = {
    versao: (publicada()?.versao ?? 0) + 1,
    publicada_por: 'Ana Ribeiro',
    publicada_em: new Date().toISOString(),
    revertida_de: revertidaDe,
    comentario: comentario ?? '',
    conteudo: copiar(conteudo),
  }
  versoes.unshift(nova)
  return nova
}

const naoEncontrado = () => HttpResponse.json({ erro: 'nao_encontrado' }, { status: 404 })

export const handlers: HttpHandler[] = [
  http.get('*/painel/politicas', async () => {
    await delay()
    return HttpResponse.json(montarEstado())
  }),

  http.put('*/painel/politicas/rascunho', async ({ request }) => {
    estado.rascunho = (await request.json()) as Politica
    estado.atualizadoEm = new Date().toISOString()
    estado.atualizadoPor = 'Ana Ribeiro'
    await delay()
    return HttpResponse.json(montarEstado())
  }),

  http.post('*/painel/politicas/testar', async ({ request }) => {
    const corpo = (await request.json()) as {
      documento: string
      pedido: Schemas['Pedido']
      dados?: Schemas['DadosOpcionais']
      politica?: Politica
    }
    const politica = corpo.politica ?? estado.rascunho
    await delay()
    const detalhes = compilar(politica)
    if (detalhes.length > 0) return politicaInvalida(detalhes)
    const ctx = contextoDoDocumento(corpo.documento, corpo.pedido, corpo.dados?.renda)
    return HttpResponse.json(simular(politica, ctx, corpo.pedido))
  }),

  http.post('*/painel/politicas/publicar', async ({ request }) => {
    const corpo = (await request.json()) as { confirmacao?: boolean; comentario?: string }
    await delay()
    if (corpo.confirmacao !== true) return HttpResponse.json({ erro: 'corpo_invalido' }, { status: 400 })
    const detalhes = compilar(estado.rascunho)
    if (detalhes.length > 0) return politicaInvalida(detalhes)
    return HttpResponse.json(publicarComo(estado.rascunho, corpo.comentario, null), { status: 201 })
  }),

  http.get('*/painel/politicas/versoes', async () => {
    await delay()
    return HttpResponse.json({
      itens: versoes.map(({ versao, publicada_por, publicada_em, revertida_de, comentario }) => ({
        versao,
        publicada_por,
        publicada_em,
        revertida_de,
        comentario,
      })),
    })
  }),

  http.get('*/painel/politicas/versoes/:versao', ({ params }) => {
    const versao = versoes.find((v) => v.versao === Number(params.versao))
    return versao ? HttpResponse.json(versao) : naoEncontrado()
  }),

  http.post('*/painel/politicas/versoes/:versao/reverter', async ({ params, request }) => {
    const corpo = (await request.json()) as { confirmacao?: boolean }
    const origem = versoes.find((v) => v.versao === Number(params.versao))
    await delay()
    if (!origem) return naoEncontrado()
    if (corpo.confirmacao !== true) return HttpResponse.json({ erro: 'corpo_invalido' }, { status: 400 })
    const nova = publicarComo(origem.conteudo, `Reversão para a v${origem.versao}`, origem.versao)
    estado.rascunho = copiar(origem.conteudo)
    return HttpResponse.json(nova, { status: 201 })
  }),

  http.get('*/painel/politicas/diferenca', async ({ request }) => {
    const parametros = new URL(request.url).searchParams
    const de = parametros.get('de') ?? ''
    const para = parametros.get('para') ?? ''
    const antes = conteudoDe(de)
    const depois = conteudoDe(para)
    await delay()
    if (!antes || !depois) return naoEncontrado()
    return HttpResponse.json({ de: Number(de), para, mudancas: diferenca(antes, depois) })
  }),
]
