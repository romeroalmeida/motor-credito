import { Plus } from 'lucide-react'
import { AvisoErro, Campo } from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { Label } from '@/shared/components/ui/label'
import { Switch } from '@/shared/components/ui/switch'
import type { ProblemaRegra } from '@/shared/utils/erroApi'
import { formatarCentavos, formatarMultiploX10, formatarTaxaBps } from '@/shared/utils/formato'
import { type FaixaEditavel, mover, novaFaixa, problemasEm, temProblema } from '../utils/politicaEditavel'
import { CartaoRegra } from './CartaoRegra'
import { EditorCondicoes } from './EditorCondicoes'
import { EntradaCel } from './EntradaCel'
import { EntradaDecimal } from './EntradaDecimal'

interface AbaFaixasProps {
  faixas: FaixaEditavel[]
  avancado: boolean
  problemas: ProblemaRegra[]
  aoMudar: (faixas: FaixaEditavel[]) => void
}

function resumoFaixa(faixa: FaixaEditavel): string {
  if (faixa.nega) return 'Nega o pedido.'
  const valido = [faixa.taxa_bps, faixa.limite_fixo, faixa.multiplo_renda_x10].every(Number.isFinite)
  if (!valido) return 'Complete os números para ver o resumo.'
  const taxa = formatarTaxaBps(faixa.taxa_bps)
  const fixo = formatarCentavos(faixa.limite_fixo)
  if (faixa.multiplo_renda_x10 === 0) return `${taxa}, limite de ${fixo}.`
  return `${taxa}, limite de até ${formatarMultiploX10(faixa.multiplo_renda_x10)} a renda, sem passar de ${fixo} (sem renda informada: ${fixo}).`
}

function proximoRotulo(faixas: FaixaEditavel[]): string {
  const usados = new Set(faixas.map((f) => f.rotulo))
  return 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('').find((letra) => !usados.has(letra)) ?? ''
}

export function AbaFaixas({ faixas, avancado, problemas, aoMudar }: AbaFaixasProps) {
  function trocar(chave: string, mudanca: Partial<FaixaEditavel>) {
    aoMudar(faixas.map((f) => (f.chave === chave ? { ...f, ...mudanca } : f)))
  }

  const problemaDaLista = problemas.find((p) => p.caminho === 'faixas')

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        A primeira faixa que bater define taxa e limite. Pedido até o limite é aprovado; acima, vai para a
        contraproposta (aba Oferta). Sem pedido, quem cai numa faixa que aprova sai pré-aprovado com o limite.
        Nenhuma faixa batendo nega com SEM_FAIXA.
      </p>
      {problemaDaLista && <AvisoErro>{problemaDaLista.motivo}</AvisoErro>}
      {faixas.map((faixa, i) => {
        const caminho = `faixas[${i}]`
        const erroEm = (campo: string) => temProblema(problemas, `${caminho}.${campo}`)
        return (
          <CartaoRegra
            key={faixa.chave}
            titulo={`Faixa ${faixa.rotulo || i + 1}`}
            posicao={i}
            total={faixas.length}
            problemas={problemasEm(problemas, caminho)}
            aoMover={(destino) => aoMudar(mover(faixas, i, destino))}
            aoRemover={() => aoMudar(faixas.filter((f) => f.chave !== faixa.chave))}
          >
            <div className="flex flex-wrap items-end gap-4">
              <Campo rotulo="Rótulo" className="w-24">
                {(props) => (
                  <Input
                    {...props}
                    aria-invalid={erroEm('rotulo')}
                    maxLength={3}
                    className="text-center font-semibold uppercase"
                    value={faixa.rotulo}
                    onChange={(e) => trocar(faixa.chave, { rotulo: e.target.value.toUpperCase() })}
                  />
                )}
              </Campo>
              <div className="flex h-10 items-center gap-2">
                <Switch
                  id={`nega-${faixa.chave}`}
                  checked={faixa.nega}
                  onCheckedChange={(nega) => trocar(faixa.chave, { nega })}
                />
                <Label htmlFor={`nega-${faixa.chave}`}>Esta faixa nega</Label>
              </div>
            </div>
            {avancado ? (
              <EntradaCel
                valor={faixa.cel ?? ''}
                comErro={erroEm('cel')}
                aoMudar={(cel) => trocar(faixa.chave, { cel })}
              />
            ) : (
              <EditorCondicoes
                legenda="Vale quando:"
                caminho={caminho}
                condicoes={faixa.condicoes}
                problemas={problemas}
                aoMudar={(condicoes) => trocar(faixa.chave, { condicoes })}
              />
            )}
            {!faixa.nega && (
              <div className="grid gap-3 sm:grid-cols-3">
                <Campo rotulo="Taxa ao mês (%)">
                  {(props) => (
                    <EntradaDecimal
                      {...props}
                      aria-invalid={erroEm('taxa_bps')}
                      valor={faixa.taxa_bps}
                      casas={2}
                      aoMudar={(taxa_bps) => trocar(faixa.chave, { taxa_bps })}
                    />
                  )}
                </Campo>
                <Campo rotulo="Limite fixo (R$)" ajuda="Teto, e o limite quando não há renda.">
                  {(props) => (
                    <EntradaDecimal
                      {...props}
                      aria-invalid={erroEm('limite_fixo')}
                      valor={faixa.limite_fixo}
                      casas={2}
                      aoMudar={(limite_fixo) => trocar(faixa.chave, { limite_fixo })}
                    />
                  )}
                </Campo>
                <Campo rotulo="Múltiplo da renda (×)" ajuda="0 usa só o limite fixo.">
                  {(props) => (
                    <EntradaDecimal
                      {...props}
                      aria-invalid={erroEm('multiplo_renda_x10')}
                      valor={faixa.multiplo_renda_x10}
                      casas={1}
                      aoMudar={(multiplo_renda_x10) => trocar(faixa.chave, { multiplo_renda_x10 })}
                    />
                  )}
                </Campo>
              </div>
            )}
            <p className="text-sm font-medium">{resumoFaixa(faixa)}</p>
          </CartaoRegra>
        )
      })}
      <Button
        type="button"
        variant="outline"
        onClick={() => aoMudar([...faixas, novaFaixa(proximoRotulo(faixas))])}
      >
        <Plus aria-hidden="true" />
        Adicionar faixa
      </Button>
    </div>
  )
}
