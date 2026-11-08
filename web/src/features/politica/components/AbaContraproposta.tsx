import { Plus, X } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { AvisoErro, Campo, Cartao } from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { Label } from '@/shared/components/ui/label'
import { Switch } from '@/shared/components/ui/switch'
import type { Schemas } from '@/shared/types'
import type { ProblemaRegra } from '@/shared/utils/erroApi'
import { lerDecimal } from '@/shared/utils/formato'
import { erroNovoPrazo, PRAZOS_PADRAO, problemasEm, temProblema } from '../utils/politicaEditavel'
import { EntradaDecimal } from './EntradaDecimal'

// No JSON o bloco continua "contraproposta" (versões já publicadas); na tela ele se chama Oferta.
type Contraproposta = Schemas['Contraproposta']

interface AbaContrapropostaProps {
  contraproposta: Contraproposta
  problemas: ProblemaRegra[]
  aoMudar: (contraproposta: Contraproposta) => void
}

export function AbaContraproposta({ contraproposta, problemas, aoMudar }: AbaContrapropostaProps) {
  const trocar = (mudanca: Partial<Contraproposta>) => aoMudar({ ...contraproposta, ...mudanca })
  const proprios = problemasEm(problemas, 'contraproposta')

  return (
    <Cartao className="space-y-6">
      <section className="space-y-4" aria-labelledby="titulo-contraproposta">
        <div className="space-y-1">
          <h3 id="titulo-contraproposta" className="text-sm font-semibold">
            Contraproposta
          </h3>
          <p className="text-sm text-muted-foreground">
            Quando o valor pedido passa do limite da faixa, o motor pode oferecer o limite em vez de negar.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Switch
            id="contraproposta-ativa"
            checked={contraproposta.ativa}
            onCheckedChange={(ativa) => trocar({ ativa })}
          />
          <Label htmlFor="contraproposta-ativa">
            {contraproposta.ativa
              ? 'Ligada: oferece o limite da faixa'
              : 'Desligada: pedido acima do limite é negado'}
          </Label>
        </div>
        {contraproposta.ativa && (
          <div className="grid max-w-xl gap-4 sm:grid-cols-2">
            <Campo rotulo="Prazo máximo (meses)" ajuda="0 mantém o prazo pedido.">
              {(props) => (
                <EntradaDecimal
                  {...props}
                  aria-invalid={temProblema(problemas, 'contraproposta.prazo_maximo')}
                  valor={contraproposta.prazo_maximo}
                  casas={0}
                  aoMudar={(prazo_maximo) => trocar({ prazo_maximo })}
                />
              )}
            </Campo>
            <Campo rotulo="Acréscimo na taxa (% ao mês)" ajuda="Somado à taxa da faixa. Ex.: 0,50">
              {(props) => (
                <EntradaDecimal
                  {...props}
                  aria-invalid={temProblema(problemas, 'contraproposta.acrescimo_taxa_bps')}
                  valor={contraproposta.acrescimo_taxa_bps}
                  casas={2}
                  aoMudar={(acrescimo_taxa_bps) => trocar({ acrescimo_taxa_bps })}
                />
              )}
            </Campo>
          </div>
        )}
      </section>
      <PrazosOfertados
        prazos={contraproposta.prazos_ofertados ?? []}
        prazoMaximo={contraproposta.prazo_maximo}
        aoMudar={(prazos_ofertados) => trocar({ prazos_ofertados })}
      />
      {proprios.map(({ caminho, motivo }) => (
        <AvisoErro key={caminho}>{motivo}</AvisoErro>
      ))}
    </Cartao>
  )
}

interface PrazosOfertadosProps {
  prazos: number[]
  prazoMaximo: number
  aoMudar: (prazos: number[]) => void
}

/** Prazos em que o limite é simulado (consulta de limite e bloco Limite da resposta). */
function PrazosOfertados({ prazos, prazoMaximo, aoMudar }: PrazosOfertadosProps) {
  const [texto, setTexto] = useState('')
  const [erro, setErro] = useState<string>()

  function adicionar(evento: FormEvent) {
    evento.preventDefault()
    const prazo = lerDecimal(texto, 0)
    const problema = erroNovoPrazo(prazos, prazo, prazoMaximo)
    setErro(problema ?? undefined)
    if (problema) return
    aoMudar([...prazos, prazo].sort((a, b) => a - b))
    setTexto('')
  }

  return (
    <section className="space-y-3 border-t pt-5" aria-labelledby="titulo-prazos">
      <div className="space-y-1">
        <h3 id="titulo-prazos" className="text-sm font-semibold">
          Prazos ofertados
        </h3>
        <p className="text-sm text-muted-foreground">
          O limite do cliente sai simulado nestes prazos, à taxa da faixa. Até 12.
        </p>
      </div>
      {prazos.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Nenhum prazo na lista: o motor usa {PRAZOS_PADRAO.join(', ')} meses.
        </p>
      ) : (
        <ul className="flex flex-wrap gap-2" aria-label="Prazos ofertados">
          {prazos.map((prazo) => (
            <li
              key={prazo}
              className="inline-flex items-center gap-1 rounded-full bg-secondary py-1 pr-1 pl-3 text-sm font-medium tabular-nums"
            >
              {prazo} meses
              <button
                type="button"
                aria-label={`Remover ${prazo} meses`}
                className="inline-flex size-6 items-center justify-center rounded-full hover:bg-card focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
                onClick={() => aoMudar(prazos.filter((p) => p !== prazo))}
              >
                <X aria-hidden="true" className="size-3.5" />
              </button>
            </li>
          ))}
        </ul>
      )}
      <form onSubmit={adicionar} noValidate className="flex max-w-sm items-start gap-2">
        <Campo rotulo="Novo prazo (meses)" erro={erro} className="flex-1">
          {(props) => (
            <Input
              {...props}
              inputMode="numeric"
              autoComplete="off"
              value={texto}
              onChange={(e) => setTexto(e.target.value)}
            />
          )}
        </Campo>
        <Button type="submit" variant="secondary" className="mt-6">
          <Plus aria-hidden="true" />
          Adicionar
        </Button>
      </form>
    </section>
  )
}
