import { ArrowLeft } from 'lucide-react'
import { Link, useParams } from 'react-router-dom'
import {
  CabecalhoPagina,
  Carregando,
  Cartao,
  ErroCarregar,
  Etiqueta,
  FontesDados,
  ResumoAntifraude,
  ResumoDecisao,
  TituloCartao,
  TrilhaRegras,
} from '@/shared/components'
import { lerErroApi } from '@/shared/utils/erroApi'
import { formatarDataHora } from '@/shared/utils/formato'
import { useConsulta } from '../hooks/useConsultas'
import type { ConsultaDetalhe } from '../services/consultasService'

const ROTULO_DADO: Record<ConsultaDetalhe['dados_informados'][number], string> = {
  nome: 'Nome',
  nascimento: 'Nascimento',
  renda: 'Renda',
  email: 'E-mail',
  telefone: 'Telefone',
  ip: 'IP',
  device_id: 'Dispositivo',
}

const Voltar = (
  <Link
    to="/consultas"
    className="inline-flex items-center gap-1.5 rounded-lg text-sm text-muted-foreground hover:text-foreground"
  >
    <ArrowLeft aria-hidden="true" className="size-4" />
    Consultas
  </Link>
)

export function ConsultaDetalhePage() {
  const { id = '' } = useParams()
  const consulta = useConsulta(id)

  if (consulta.isPending) {
    return (
      <>
        <CabecalhoPagina titulo="Consulta" voltar={Voltar} />
        <Cartao>
          <Carregando linhas={5} />
        </Cartao>
      </>
    )
  }

  if (consulta.isError) {
    const erro = lerErroApi(consulta.error)
    return (
      <>
        <CabecalhoPagina titulo="Consulta" voltar={Voltar} />
        <Cartao>
          <ErroCarregar
            mensagem={
              erro.status === 404 ? 'Esta consulta não existe ou não é da sua empresa.' : erro.mensagem
            }
            aoTentarDeNovo={erro.status === 404 ? undefined : () => consulta.refetch()}
          />
        </Cartao>
      </>
    )
  }

  return <Detalhe consulta={consulta.data} />
}

function Detalhe({ consulta }: { consulta: ConsultaDetalhe }) {
  return (
    <>
      <CabecalhoPagina
        titulo={consulta.documento_mascarado}
        voltar={Voltar}
        descricao={
          <>
            {consulta.tipo_documento?.toUpperCase()} · {formatarDataHora(consulta.criado_em)} ·{' '}
            <span className="font-mono">{consulta.id}</span>
          </>
        }
      />
      <div className="grid gap-6 lg:grid-cols-3">
        <div className="space-y-6 lg:col-span-2">
          <Cartao aria-labelledby="titulo-decisao">
            <TituloCartao id="titulo-decisao">Decisão</TituloCartao>
            <ResumoDecisao
              status={consulta.status}
              score={consulta.score}
              faixaRisco={consulta.faixa_risco}
              pedido={consulta.pedido}
              oferta={consulta.oferta}
              motivos={consulta.motivos}
            />
          </Cartao>
          <Cartao aria-labelledby="titulo-trilha">
            <TituloCartao
              id="titulo-trilha"
              descricao={
                consulta.politica
                  ? `Avaliadas com a política versão ${consulta.politica.versao}, na ordem em que o motor percorreu.`
                  : 'Na ordem em que o motor percorreu.'
              }
            >
              Regras avaliadas
            </TituloCartao>
            <TrilhaRegras trilha={consulta.trilha} />
          </Cartao>
        </div>
        <div className="space-y-6">
          <Cartao aria-labelledby="titulo-fontes">
            <TituloCartao id="titulo-fontes">De onde vêm os dados</TituloCartao>
            <FontesDados
              fontes={consulta.fontes}
              fatores={consulta.score_fatores}
              cadastro={consulta.cadastro}
            />
          </Cartao>
          <Cartao aria-labelledby="titulo-antifraude">
            <TituloCartao id="titulo-antifraude">Antifraude</TituloCartao>
            <ResumoAntifraude antifraude={consulta.antifraude} />
          </Cartao>
          <Cartao aria-labelledby="titulo-contexto">
            <TituloCartao id="titulo-contexto">Contexto</TituloCartao>
            <dl className="space-y-3 text-sm">
              <div>
                <dt className="text-xs text-muted-foreground">Política</dt>
                <dd className="font-medium">
                  {consulta.politica ? `Versão ${consulta.politica.versao}` : 'Não chegou à política'}
                </dd>
              </div>
              <div>
                <dt className="text-xs text-muted-foreground">Finalidade</dt>
                <dd>
                  {consulta.finalidade === 'concessao_credito' ? 'Concessão de crédito' : consulta.finalidade}
                </dd>
              </div>
              <div>
                <dt className="text-xs text-muted-foreground">Base legal (LGPD)</dt>
                <dd>{consulta.base_legal}</dd>
              </div>
              <div>
                <dt className="text-xs text-muted-foreground">Dados informados</dt>
                <dd className="mt-1.5">
                  {consulta.dados_informados.length === 0 ? (
                    'Só o documento'
                  ) : (
                    <ul className="flex flex-wrap gap-1.5">
                      {consulta.dados_informados.map((dado) => (
                        <li key={dado}>
                          <Etiqueta>{ROTULO_DADO[dado]}</Etiqueta>
                        </li>
                      ))}
                    </ul>
                  )}
                </dd>
              </div>
            </dl>
          </Cartao>
        </div>
      </div>
    </>
  )
}
