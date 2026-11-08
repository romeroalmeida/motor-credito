import { AvisoErro, Campo, Cartao } from '@/shared/components'
import { Label } from '@/shared/components/ui/label'
import { Switch } from '@/shared/components/ui/switch'
import type { Schemas } from '@/shared/types'
import type { ProblemaRegra } from '@/shared/utils/erroApi'
import { problemasEm, temProblema } from '../utils/politicaEditavel'
import { EntradaDecimal } from './EntradaDecimal'

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
    <Cartao className="space-y-5">
      <p className="text-sm text-muted-foreground">
        Quando o valor pedido passa do limite da faixa, o motor pode oferecer o limite em vez de negar.
      </p>
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
      {proprios.map(({ caminho, motivo }) => (
        <AvisoErro key={caminho}>{motivo}</AvisoErro>
      ))}
    </Cartao>
  )
}
