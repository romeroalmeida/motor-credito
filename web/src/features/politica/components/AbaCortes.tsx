import { Plus, ShieldCheck } from 'lucide-react'
import { Campo, Vazio } from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import type { ProblemaRegra } from '@/shared/utils/erroApi'
import { type CorteEditavel, mover, novoCorte, problemasEm, temProblema } from '../utils/politicaEditavel'
import { CartaoRegra } from './CartaoRegra'
import { EditorCondicoes } from './EditorCondicoes'
import { EntradaCel } from './EntradaCel'

interface AbaCortesProps {
  cortes: CorteEditavel[]
  avancado: boolean
  problemas: ProblemaRegra[]
  aoMudar: (cortes: CorteEditavel[]) => void
}

export function AbaCortes({ cortes, avancado, problemas, aoMudar }: AbaCortesProps) {
  function trocar(chave: string, mudanca: Partial<CorteEditavel>) {
    aoMudar(cortes.map((c) => (c.chave === chave ? { ...c, ...mudanca } : c)))
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        Avaliados de cima para baixo. O primeiro que bater nega o pedido com o código dele, antes de olhar as
        faixas.
      </p>
      {cortes.length === 0 && (
        <Vazio icone={ShieldCheck} titulo="Nenhum corte">
          Sem cortes, todo pedido segue direto para as faixas.
        </Vazio>
      )}
      {cortes.map((corte, i) => {
        const caminho = `cortes[${i}]`
        return (
          <CartaoRegra
            key={corte.chave}
            titulo={`Corte ${corte.codigo || i + 1}`}
            posicao={i}
            total={cortes.length}
            problemas={problemasEm(problemas, caminho)}
            aoMover={(destino) => aoMudar(mover(cortes, i, destino))}
            aoRemover={() => aoMudar(cortes.filter((c) => c.chave !== corte.chave))}
          >
            <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
              <Campo rotulo="Código" ajuda="Volta na resposta da API, ex.: FRAUDE_ALTA">
                {(props) => (
                  <Input
                    {...props}
                    aria-invalid={temProblema(problemas, `${caminho}.codigo`)}
                    className="font-mono uppercase"
                    value={corte.codigo}
                    onChange={(e) =>
                      trocar(corte.chave, { codigo: e.target.value.toUpperCase().replace(/\s/g, '_') })
                    }
                  />
                )}
              </Campo>
              <Campo rotulo="Motivo" ajuda="Texto do motivo devolvido ao cliente.">
                {(props) => (
                  <Input
                    {...props}
                    aria-invalid={temProblema(problemas, `${caminho}.texto`)}
                    value={corte.texto}
                    onChange={(e) => trocar(corte.chave, { texto: e.target.value })}
                  />
                )}
              </Campo>
            </div>
            {avancado ? (
              <EntradaCel
                valor={corte.cel ?? ''}
                comErro={temProblema(problemas, `${caminho}.cel`)}
                aoMudar={(cel) => trocar(corte.chave, { cel })}
              />
            ) : (
              <EditorCondicoes
                legenda="Nega quando:"
                caminho={caminho}
                condicoes={corte.condicoes}
                problemas={problemas}
                aoMudar={(condicoes) => trocar(corte.chave, { condicoes })}
              />
            )}
          </CartaoRegra>
        )
      })}
      <Button type="button" variant="outline" onClick={() => aoMudar([...cortes, novoCorte()])}>
        <Plus aria-hidden="true" />
        Adicionar corte
      </Button>
    </div>
  )
}
