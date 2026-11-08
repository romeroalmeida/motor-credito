import { CircleCheck, CircleMinus } from 'lucide-react'
import type { ReactNode } from 'react'
import { Etiqueta, type Tom } from '@/shared/components/Etiqueta'
import { StatusDecisao } from '@/shared/components/StatusDecisao'
import { cn } from '@/shared/lib/utils'
import type { Schemas } from '@/shared/types'
import { formatarCentavos, formatarTaxaBps } from '@/shared/utils/formato'
import { rotuloSinal } from '@/shared/utils/rotulos'

// Partes da decisão que aparecem no detalhe da consulta e no teste da política.

type Antifraude = Schemas['Antifraude']
type Checagem = Schemas['Checagem']

const ROTULO_CHECAGEM: Record<Checagem, string> = {
  documento: 'Documento',
  velocidade: 'Velocidade',
  nome: 'Nome',
  nascimento: 'Nascimento',
  email: 'E-mail',
  ip: 'IP',
  dispositivo: 'Dispositivo',
}

const ROTULO_BLOCO: Record<Schemas['RegraAvaliada']['bloco'], string> = {
  corte: 'Corte',
  faixa: 'Faixa',
  contraproposta: 'Contraproposta',
}

/** Risco a partir do qual o antifraude pede atenção (mesmo corte do modelo PF: fraude.risco >= 70). */
const RISCO_ALTO = 70

function Dado({ rotulo, children }: { rotulo: string; children: ReactNode }) {
  return (
    <div className="min-w-0 space-y-1">
      <dt className="text-xs text-muted-foreground">{rotulo}</dt>
      <dd className="font-medium tabular-nums">{children}</dd>
    </div>
  )
}

interface ResumoDecisaoProps {
  status: Schemas['StatusDecisao']
  score?: number | null
  faixaRisco?: string | null
  pedido?: Schemas['Pedido']
  oferta?: Schemas['Oferta'] | null
  motivos: Schemas['Motivo'][]
}

export function ResumoDecisao({ status, score, faixaRisco, pedido, oferta, motivos }: ResumoDecisaoProps) {
  return (
    <div className="@container space-y-5">
      <StatusDecisao status={status} className="text-base" />
      <dl className="grid grid-cols-2 gap-4 @lg:grid-cols-4">
        <Dado rotulo="Score">{score ?? '—'}</Dado>
        <Dado rotulo="Faixa de risco">{faixaRisco ?? '—'}</Dado>
        {pedido && (
          <Dado rotulo="Pedido">
            {formatarCentavos(pedido.valor)} em {pedido.prazo}×
          </Dado>
        )}
        {oferta && (
          <>
            <Dado rotulo="Oferta">
              {formatarCentavos(oferta.valor)} em {oferta.prazo}×
            </Dado>
            <Dado rotulo="Taxa">{formatarTaxaBps(oferta.taxa_mensal_bps)}</Dado>
            <Dado rotulo="Parcela">{formatarCentavos(oferta.parcela)}</Dado>
          </>
        )}
      </dl>
      {motivos.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-sm font-semibold">Motivos</h3>
          <ul className="space-y-1.5">
            {motivos.map((motivo) => (
              <li key={motivo.codigo} className="text-sm">
                <code className="mr-2 rounded-md bg-secondary px-1.5 py-0.5 font-mono text-xs">
                  {motivo.codigo}
                </code>
                {motivo.texto}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

export function TrilhaRegras({ trilha }: { trilha: Schemas['RegraAvaliada'][] }) {
  if (trilha.length === 0) {
    return <p className="text-sm text-muted-foreground">Nenhuma regra foi avaliada nesta decisão.</p>
  }
  return (
    <ol className="space-y-2" aria-label="Regras avaliadas, na ordem">
      {trilha.map((regra, indice) => {
        const Icone = regra.bateu ? CircleCheck : CircleMinus
        return (
          <li
            key={`${regra.bloco}-${regra.rotulo}-${indice}`}
            className={cn('flex gap-3 rounded-xl p-3', regra.bateu ? 'bg-secondary' : 'bg-card')}
          >
            <Icone
              aria-hidden="true"
              className={cn(
                'mt-0.5 size-4 shrink-0',
                regra.bateu ? 'text-foreground' : 'text-muted-foreground',
              )}
            />
            <div className="min-w-0 flex-1 space-y-1">
              <p className="flex flex-wrap items-baseline gap-x-2 text-sm">
                <span className="font-medium">
                  {ROTULO_BLOCO[regra.bloco]} {regra.rotulo}
                </span>
                <span className={cn('text-xs', regra.bateu ? 'font-semibold' : 'text-muted-foreground')}>
                  {regra.bateu ? 'bateu' : 'não bateu'}
                </span>
              </p>
              <code className="block font-mono text-xs break-words text-muted-foreground">
                {regra.expressao}
              </code>
            </div>
          </li>
        )
      })}
    </ol>
  )
}

export function ResumoAntifraude({ antifraude }: { antifraude: Antifraude | null | undefined }) {
  if (!antifraude) {
    return <p className="text-sm text-muted-foreground">O antifraude não rodou nesta decisão.</p>
  }
  const alto = antifraude.risco >= RISCO_ALTO
  return (
    <div className="space-y-4">
      <p className="flex items-baseline gap-2">
        <span className="text-3xl font-semibold tracking-tight tabular-nums">{antifraude.risco}</span>
        <span className="text-sm text-muted-foreground">de 100 de risco</span>
        {alto && <Etiqueta tom="alerta">Risco alto</Etiqueta>}
      </p>
      <div className="space-y-2">
        <h3 className="text-sm font-semibold">Sinais</h3>
        {antifraude.sinais.length === 0 ? (
          <p className="text-sm text-muted-foreground">Nenhum sinal levantado.</p>
        ) : (
          <ul className="flex flex-wrap gap-2">
            {antifraude.sinais.map((sinal) => (
              <li key={sinal}>
                <Etiqueta tom="alerta">{rotuloSinal(sinal)}</Etiqueta>
              </li>
            ))}
          </ul>
        )}
      </div>
      <dl className="grid gap-3 text-sm sm:grid-cols-2">
        <div>
          <dt className="text-xs text-muted-foreground">Checagens feitas</dt>
          <dd>{antifraude.checagens_executadas.map((c) => ROTULO_CHECAGEM[c]).join(', ') || '—'}</dd>
        </div>
        <div>
          <dt className="text-xs text-muted-foreground">Puladas por falta de dado</dt>
          <dd>{antifraude.checagens_puladas.map((c) => ROTULO_CHECAGEM[c]).join(', ') || '—'}</dd>
        </div>
      </dl>
    </div>
  )
}

type Fontes = Schemas['Fontes']

/** Selo de cada fonte: o texto diz se o dado é real ou simulado; a cor só reforça. */
const SELO_CADASTRO: Record<Fontes['cadastro'], { texto: string; tom: Tom }> = {
  serpro: { texto: 'Real · Serpro', tom: 'aprovado' },
  'serpro-trial': { texto: 'Real · Serpro (ambiente de testes)', tom: 'aprovado' },
  brasilapi: { texto: 'Real · BrasilAPI', tom: 'aprovado' },
  simulado: { texto: 'Simulado', tom: 'neutro' },
  nao_verificado: { texto: 'Não verificado', tom: 'alerta' },
}
const SELO_SCORE: Record<Fontes['score'], { texto: string; tom: Tom }> = {
  cadastral: { texto: 'Cadastral (dados públicos)', tom: 'aprovado' },
  simulado: { texto: 'Simulado', tom: 'neutro' },
}
const SELO_RESTRICOES: Record<Fontes['restricoes'], { texto: string; tom: Tom }> = {
  simulado: { texto: 'Simulado', tom: 'neutro' },
}
const ROTULO_SITUACAO: Record<Schemas['Cadastro']['situacao'], string> = {
  regular: 'Regular',
  irregular: 'Irregular',
  inativo: 'Inativa',
}

/** "1966-08-01" → "01/08/1966" sem passar por Date (que mudaria o dia pelo fuso). */
const dataCurta = (iso: string) => iso.split('-').reverse().join('/')

interface FontesDadosProps {
  fontes?: Fontes | null
  fatores?: Schemas['FatorScore'][] | null
  cadastro?: Schemas['Cadastro'] | null
}

export function FontesDados({ fontes, fatores, cadastro }: FontesDadosProps) {
  if (!fontes) {
    return <p className="text-sm text-muted-foreground">Nenhuma fonte de dados respondeu nesta decisão.</p>
  }
  const selos = [
    { rotulo: 'Cadastro', ...SELO_CADASTRO[fontes.cadastro] },
    { rotulo: 'Score', ...SELO_SCORE[fontes.score] },
    { rotulo: 'Restrições e protestos', ...SELO_RESTRICOES[fontes.restricoes] },
  ]
  return (
    <div className="space-y-4">
      <dl className="space-y-2 text-sm">
        {selos.map((selo) => (
          <div key={selo.rotulo} className="flex flex-wrap items-center justify-between gap-2">
            <dt className="text-muted-foreground">{selo.rotulo}</dt>
            <dd>
              <Etiqueta tom={selo.tom}>{selo.texto}</Etiqueta>
            </dd>
          </div>
        ))}
      </dl>
      {fontes.cadastro === 'nao_verificado' && (
        <p className="text-xs text-muted-foreground">
          O Serpro exige o nascimento: sem ele a situação do CPF não foi consultada nem pontuada.
        </p>
      )}
      {cadastro && (
        <dl className="grid gap-3 text-sm sm:grid-cols-2">
          <Dado rotulo="Situação cadastral">{ROTULO_SITUACAO[cadastro.situacao]}</Dado>
          {cadastro.razao_social && <Dado rotulo="Razão social">{cadastro.razao_social}</Dado>}
          {cadastro.abertura && <Dado rotulo="Abertura">{dataCurta(cadastro.abertura)}</Dado>}
        </dl>
      )}
      {fatores && fatores.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-sm font-semibold">Como o score cadastral foi somado</h3>
          <ul className="space-y-1" aria-label="Fatores do score cadastral">
            {fatores.map((fator) => (
              <li key={fator.codigo} className="flex justify-between gap-3 text-sm">
                <code className="font-mono text-xs">{fator.codigo}</code>
                <span className="tabular-nums">
                  {fator.pontos >= 0 ? '+' : ''}
                  {fator.pontos}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
