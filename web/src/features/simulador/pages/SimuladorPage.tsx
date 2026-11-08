import { KeyRound } from 'lucide-react'
import { Link } from 'react-router-dom'
import { AvisoErro, CabecalhoPagina, Carregando, Cartao, ErroCarregar, Vazio } from '@/shared/components'
import { buttonVariants } from '@/shared/components/ui/button'
import { lerErroApi } from '@/shared/utils/erroApi'
import { CartaoCodigo } from '../components/CartaoCodigo'
import { CartaoRequisicao } from '../components/CartaoRequisicao'
import { CartaoResposta } from '../components/CartaoResposta'
import { FormSimulador } from '../components/FormSimulador'
import { useChavesAtivas, useSimular } from '../hooks/useSimulador'

export function SimuladorPage() {
  const chaves = useChavesAtivas()
  const simular = useSimular()
  const resultado = simular.data

  return (
    <>
      <CabecalhoPagina
        titulo="Simulador de integração"
        descricao="Monte uma consulta, veja a requisição assinada que a API recebe, a resposta real e o código para levar ao seu sistema."
      />
      {chaves.isPending ? (
        <Cartao>
          <Carregando linhas={5} rotulo="Carregando chaves…" />
        </Cartao>
      ) : chaves.isError ? (
        <Cartao>
          <ErroCarregar
            mensagem={lerErroApi(chaves.error).mensagem}
            aoTentarDeNovo={() => chaves.refetch()}
          />
        </Cartao>
      ) : chaves.data.length === 0 ? (
        <Cartao>
          <Vazio
            icone={KeyRound}
            titulo="Nenhuma chave de API ativa"
            acao={
              <Link to="/chaves" className={buttonVariants()}>
                Criar uma chave
              </Link>
            }
          >
            O simulador assina com uma chave do seu tenant. Crie uma e volte aqui.
          </Vazio>
        </Cartao>
      ) : (
        <div className="grid gap-4 xl:grid-cols-[minmax(0,26rem)_minmax(0,1fr)] xl:items-start">
          <FormSimulador
            chaves={chaves.data}
            enviando={simular.isPending}
            ultimaIdem={resultado?.requisicao.cabecalhos['Idempotency-Key'] ?? null}
            aoEnviar={(entrada) => simular.mutate(entrada)}
          />
          <section
            aria-label="Resultado"
            aria-live="polite"
            aria-busy={simular.isPending}
            className="min-w-0 space-y-4"
          >
            {simular.isError && <ErroSimulador erro={simular.error} />}
            {simular.isPending && (
              <Cartao>
                <Carregando linhas={4} rotulo="Enviando a consulta…" />
              </Cartao>
            )}
            {resultado && !simular.isPending && (
              <>
                <CartaoRequisicao requisicao={resultado.requisicao} />
                <CartaoResposta resposta={resultado.resposta} consultaId={resultado.consulta_id} />
                <CartaoCodigo trechos={resultado.trechos} />
              </>
            )}
          </section>
        </div>
      )}
    </>
  )
}

/** Erro do próprio painel (a /v1 não chegou a ser chamada): os da /v1 aparecem no cartão Resposta. */
function ErroSimulador({ erro }: { erro: unknown }) {
  const { status, mensagem } = lerErroApi(erro)
  return (
    <Cartao>
      <AvisoErro>
        {status === 404
          ? 'Esta chave não existe mais ou foi revogada. Recarregue a página e escolha outra.'
          : mensagem}
      </AvisoErro>
    </Cartao>
  )
}
