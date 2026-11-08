import { delay, type HttpHandler, http, HttpResponse } from 'msw'
import type { ApiPaths, Schemas } from '@/shared/types'

// Handlers MSW da feature, montados em src/mocks/handlers.ts (dev e Vitest).
// 40 consultas determinísticas com os status que a v1 produz (pré-aprovado = consulta sem pedido).

type Resumo = Schemas['ConsultaResumo']
type Detalhe = Schemas['ConsultaDetalhe']

/** CPF de teste com DV válido: o único que a busca por documento encontra. */
export const CPF_BUSCAVEL = '52998224725'

const AGORA = Date.parse('2026-11-07T14:00:00Z')
const MINUTO = 60_000
const STATUS: Resumo['status'][] = [
  'aprovado',
  'negado',
  'contraproposta',
  'aprovado',
  'documento_invalido',
  'aprovado',
  'negado',
  'contraproposta',
  'pre_aprovado',
]
const PEDIDOS = [500000, 1000000, 250000, 1500000, 800000, 300000]
const PRAZOS = [12, 24, 36, 18]

function mascara(i: number): { documento_mascarado: string; tipo_documento: 'cpf' | 'cnpj' } {
  if (i === 0) return { documento_mascarado: '***.982.247-**', tipo_documento: 'cpf' }
  if (i % 7 === 3) return { documento_mascarado: '**.ABC.345/****-**', tipo_documento: 'cnpj' }
  const meio = String(100000 + ((i * 7919) % 900000))
  return { documento_mascarado: `***.${meio.slice(0, 3)}.${meio.slice(3)}-**`, tipo_documento: 'cpf' }
}

// Parcela pela Price em float: aceitável só no mock; o servidor calcula em big.Rat.
function price(valor: number, prazo: number, taxaBps: number): number {
  const i = taxaBps / 10000
  return Math.round((valor * i) / (1 - (1 + i) ** -prazo))
}

function faixaDoScore(score: number): string {
  if (score <= 300) return 'C'
  if (score <= 600) return 'B'
  return 'A'
}

function montarResumo(i: number): Resumo {
  const status = STATUS[i % STATUS.length] ?? 'aprovado'
  const pedido =
    status === 'pre_aprovado'
      ? null
      : { valor: PEDIDOS[i % PEDIDOS.length] ?? 500000, prazo: PRAZOS[i % PRAZOS.length] ?? 12 }
  const invalido = status === 'documento_invalido'
  const score = invalido ? null : status === 'negado' ? 150 + ((i * 37) % 150) : 420 + ((i * 53) % 560)
  const taxa = score !== null && score > 600 ? 290 : 590
  const oferta = !pedido
    ? null
    : status === 'aprovado'
      ? {
          valor: pedido.valor,
          prazo: pedido.prazo,
          taxa_mensal_bps: taxa,
          parcela: price(pedido.valor, pedido.prazo, taxa),
        }
      : status === 'contraproposta'
        ? {
            valor: Math.round(pedido.valor * 0.6),
            prazo: pedido.prazo,
            taxa_mensal_bps: taxa,
            parcela: price(Math.round(pedido.valor * 0.6), pedido.prazo, taxa),
          }
        : null
  return {
    id: `cons_01JC${String(i).padStart(4, '0')}MOCK0000000000000`,
    ...mascara(i),
    status,
    score,
    faixa_risco: score === null ? null : faixaDoScore(score),
    pedido,
    oferta,
    antifraude_risco: invalido ? null : (i * 13) % 85,
    politica_versao: invalido ? null : i < 12 ? 3 : 2,
    criado_em: new Date(AGORA - i * 97 * MINUTO).toISOString(),
  }
}

export const CONSULTAS: Resumo[] = Array.from({ length: 40 }, (_, i) => montarResumo(i))

const MOTIVOS: Record<Resumo['status'], Schemas['Motivo'][]> = {
  aprovado: [],
  contraproposta: [{ codigo: 'LIMITE_FAIXA', texto: 'Valor pedido acima do limite para o score' }],
  negado: [{ codigo: 'FAIXA_NEGADA', texto: 'A faixa C nega o pedido' }],
  pre_aprovado: [],
  documento_invalido: [{ codigo: 'DOCUMENTO_INVALIDO', texto: 'Documento com dígito verificador inválido' }],
  em_analise: [],
}

function trilhaPara(resumo: Resumo): Schemas['RegraAvaliada'][] {
  const score = resumo.score
  if (score === null || score === undefined) return []
  const cortes: Schemas['RegraAvaliada'][] = [
    { bloco: 'corte', rotulo: 'FRAUDE_ALTA', expressao: 'fraude.risco >= 70', bateu: false },
    { bloco: 'corte', rotulo: 'RESTRICOES', expressao: 'bureau.restricoes > 2', bateu: false },
    { bloco: 'corte', rotulo: 'MENOR_IDADE', expressao: 'has(idade) && idade < 18', bateu: false },
  ]
  const faixas: Schemas['RegraAvaliada'][] = [
    { bloco: 'faixa', rotulo: 'C', expressao: 'score <= 300', bateu: score <= 300 },
    {
      bloco: 'faixa',
      rotulo: 'B',
      expressao: 'score > 300 && score <= 600',
      bateu: score > 300 && score <= 600,
    },
    { bloco: 'faixa', rotulo: 'A', expressao: 'score > 600', bateu: score > 600 },
  ]
  // A trilha para na primeira faixa que bate, como o motor.
  const ate = faixas.findIndex((f) => f.bateu)
  const trilha = [...cortes, ...faixas.slice(0, ate + 1)]
  if (resumo.status === 'contraproposta') {
    trilha.push({ bloco: 'contraproposta', rotulo: 'ativa', expressao: 'pedido.valor > limite', bateu: true })
  }
  return trilha
}

/** Fatores do score cadastral de um CNPJ de exemplo (mesma tabela do internal/bureau). */
const FATORES_EXEMPLO: Schemas['FatorScore'][] = [
  { codigo: 'SITUACAO_ATIVA', pontos: 250 },
  { codigo: 'TEMPO_EMPRESA_5A_MAIS', pontos: 200 },
  { codigo: 'CAPITAL_100MIL_1MI', pontos: 100 },
  { codigo: 'PORTE_PEQUENO', pontos: 70 },
  { codigo: 'REGIME_SIMPLES', pontos: 50 },
  { codigo: 'NATUREZA_LIMITADA', pontos: 100 },
  { codigo: 'SOCIOS_2_MAIS', pontos: 50 },
]

// CNPJ: cadastro e score reais pela BrasilAPI; o primeiro CPF pelo trial do Serpro; o resto simulado.
function fontesPara(resumo: Resumo, i: number): Pick<Detalhe, 'fontes' | 'score_fatores' | 'cadastro'> {
  if (resumo.status === 'documento_invalido') return { fontes: null, score_fatores: null, cadastro: null }
  if (resumo.tipo_documento === 'cnpj')
    return {
      fontes: { cadastro: 'brasilapi', score: 'cadastral', restricoes: 'simulado' },
      score_fatores: FATORES_EXEMPLO,
      cadastro: { situacao: 'regular', razao_social: 'PADARIA EXEMPLO LTDA', abertura: '2012-03-15' },
    }
  return {
    fontes: { cadastro: i === 0 ? 'serpro-trial' : 'simulado', score: 'simulado', restricoes: 'simulado' },
    score_fatores: null,
    cadastro: { situacao: 'regular' },
  }
}

const STATUS_COM_LIMITE: Resumo['status'][] = ['aprovado', 'contraproposta', 'pre_aprovado']

function limitePara(resumo: Resumo): Schemas['Limite'] | null {
  if (!STATUS_COM_LIMITE.includes(resumo.status)) return null
  const taxa = (resumo.score ?? 0) > 600 ? 290 : 590
  const valor = taxa === 290 ? 3_000_000 : 1_000_000
  return {
    valor_maximo: valor,
    taxa_mensal_bps: taxa,
    simulacoes: [12, 24, 36].map((prazo) => ({ prazo, parcela: price(valor, prazo, taxa) })),
  }
}

function montarDetalhe(resumo: Resumo, i: number): Detalhe {
  const invalido = resumo.status === 'documento_invalido'
  const comEmail = i % 3 !== 0
  return {
    id: resumo.id,
    status: resumo.status,
    score: resumo.score,
    faixa_risco: resumo.faixa_risco,
    oferta: resumo.oferta,
    limite: limitePara(resumo),
    pedido: resumo.pedido,
    motivos: MOTIVOS[resumo.status],
    antifraude: invalido
      ? null
      : {
          risco: resumo.antifraude_risco ?? 0,
          sinais: (resumo.antifraude_risco ?? 0) >= 40 ? ['email_descartavel', 'ip_datacenter'] : [],
          checagens_executadas: comEmail
            ? ['documento', 'velocidade', 'email', 'ip']
            : ['documento', 'velocidade'],
          checagens_puladas: comEmail
            ? ['nome', 'nascimento', 'dispositivo']
            : ['nome', 'nascimento', 'email', 'ip', 'dispositivo'],
        },
    politica: resumo.politica_versao
      ? { id: 'pol_01JC8ZK3Q0EMPRESTIMOPF00', versao: resumo.politica_versao }
      : null,
    criado_em: resumo.criado_em,
    documento_mascarado: resumo.documento_mascarado,
    tipo_documento: resumo.tipo_documento,
    finalidade: 'concessao_credito',
    base_legal: 'Proteção do crédito (LGPD, art. 7º, X)',
    dados_informados: comEmail ? ['renda', 'email', 'ip'] : [],
    trilha: trilhaPara(resumo),
    ...fontesPara(resumo, i),
  }
}

type Busca = ApiPaths['/painel/consultas/busca']['post']['requestBody']['content']['application/json']

function filtrar({ status, faixa, documento, desde, ate }: Busca): Resumo[] {
  return CONSULTAS.filter(
    (c, i) =>
      (!status || c.status === status) &&
      (!faixa || c.faixa_risco === faixa) &&
      (!documento || (documento === CPF_BUSCAVEL && i === 0)) &&
      (!desde || c.criado_em.slice(0, 10) >= desde) &&
      (!ate || c.criado_em.slice(0, 10) <= ate),
  )
}

export const handlers: HttpHandler[] = [
  http.post('*/painel/consultas/busca', async ({ request }) => {
    const busca = (await request.json()) as Busca
    const limite = busca.limite ?? 25
    const inicio = Number(busca.cursor ?? 0)
    const encontradas = filtrar(busca)
    const proximo = inicio + limite < encontradas.length ? String(inicio + limite) : null
    await delay()
    return HttpResponse.json({ itens: encontradas.slice(inicio, inicio + limite), proximo_cursor: proximo })
  }),

  http.get('*/painel/consultas/:id', async ({ params }) => {
    const i = CONSULTAS.findIndex((c) => c.id === params.id)
    await delay()
    const resumo = CONSULTAS[i]
    if (!resumo) return HttpResponse.json({ erro: 'nao_encontrado' }, { status: 404 })
    return HttpResponse.json(montarDetalhe(resumo, i))
  }),
]
