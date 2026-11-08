import { ExternalLink } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Cartao, Etiqueta, FontesDados, StatusDecisao, TituloCartao } from '@/shared/components'
import type { Schemas } from '@/shared/types'
import type { ResultadoSimulador } from '../services/simuladorService'
import { jsonLegivel } from '../utils/jsonLegivel'
import { BlocoCodigo, BotaoCopiar } from './BotaoCopiar'

type Resposta = ResultadoSimulador['resposta']

const ehObjeto = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null

export function CartaoResposta({ resposta, consultaId }: { resposta: Resposta; consultaId: string | null }) {
  const ok = resposta.status >= 200 && resposta.status < 300
  const decisao = ok && ehObjeto(resposta.corpo) ? (resposta.corpo as Schemas['ConsultaResposta']) : null
  const json = jsonLegivel(resposta.corpo)
  return (
    <Cartao aria-labelledby="titulo-resposta">
      <TituloCartao
        id="titulo-resposta"
        descricao="O que a API pública devolveu, sem retoque."
        acoes={
          consultaId && (
            <Link
              to={`/consultas/${encodeURIComponent(consultaId)}`}
              className="inline-flex items-center gap-1.5 text-sm font-medium underline-offset-4 hover:underline"
            >
              Ver no painel
              <ExternalLink aria-hidden="true" className="size-3.5" />
            </Link>
          )
        }
      >
        Resposta
      </TituloCartao>
      <dl className="mb-4 flex flex-wrap items-center gap-x-6 gap-y-3 text-sm">
        <div className="flex items-center gap-2">
          <dt className="text-muted-foreground">Status HTTP</dt>
          <dd>
            <Etiqueta tom={ok ? 'aprovado' : resposta.status === 429 ? 'alerta' : 'negado'}>
              <span className="font-mono">{resposta.status}</span>
            </Etiqueta>
          </dd>
        </div>
        <div className="flex items-center gap-2">
          <dt className="text-muted-foreground">Duração</dt>
          <dd className="tabular-nums">{resposta.duracao_ms} ms</dd>
        </div>
        {decisao && (
          <div className="flex items-center gap-2">
            <dt className="text-muted-foreground">Decisão</dt>
            <dd>
              <StatusDecisao status={decisao.status} />
            </dd>
          </div>
        )}
      </dl>
      <ExplicacaoErro resposta={resposta} />
      {decisao && (
        <div className="mb-5 space-y-2">
          <h3 className="text-sm font-semibold">De onde vêm os dados</h3>
          <FontesDados fontes={decisao.fontes} fatores={decisao.score_fatores} cadastro={decisao.cadastro} />
        </div>
      )}
      <div className="space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-sm font-semibold">Corpo</h3>
          <BotaoCopiar texto={json} rotulo="Copiar corpo da resposta" />
        </div>
        <BlocoCodigo codigo={json} rotulo="Corpo da resposta em JSON" />
      </div>
    </Cartao>
  )
}

/** A API não diz qual parte da credencial falhou; a tela lista as causas prováveis. */
function ExplicacaoErro({ resposta }: { resposta: Resposta }) {
  const corpo = ehObjeto(resposta.corpo) ? (resposta.corpo as Partial<Schemas['Erro']>) : {}
  let texto: ReactNode = null
  switch (resposta.status) {
    case 400:
      texto = (
        <>
          A API recusou o corpo.
          {corpo.campos && (
            <ul className="mt-2 list-disc space-y-1 pl-5">
              {Object.entries(corpo.campos).map(([campo, motivo]) => (
                <li key={campo}>
                  <code className="font-mono text-xs">{campo}</code>: {motivo}
                </li>
              ))}
            </ul>
          )}
        </>
      )
      break
    case 401:
      texto = (
        <>
          A API recusou a credencial e, por segurança, não diz qual parte falhou. Causas prováveis: a chave
          foi revogada há pouco; o seu IP (o desta sessão do painel) não está na{' '}
          <Link to="/chaves" className="font-medium underline underline-offset-4">
            lista de IPs permitidos
          </Link>
          ; ou o relógio do servidor diverge mais de 5 minutos.
        </>
      )
      break
    case 409:
      texto =
        'Esta Idempotency-Key já foi usada com outro corpo. Gere uma nova, ou reenvie exatamente o mesmo pedido para receber a mesma decisão.'
      break
    case 429: {
      const espera = resposta.cabecalhos['Retry-After']
      texto = `Limite de consultas por minuto do tenant atingido.${espera ? ` Tente de novo em ${espera} s.` : ''}`
      break
    }
  }
  if (!texto) return null
  return (
    <div role="status" className="mb-5 rounded-xl bg-secondary p-3 text-sm">
      {texto}
    </div>
  )
}
