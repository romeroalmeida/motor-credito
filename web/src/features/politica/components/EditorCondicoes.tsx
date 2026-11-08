import { Plus, Trash2 } from 'lucide-react'
import { Campo, CLASSE_SELECT } from '@/shared/components'
import { ROTULO_SINAL } from '@/shared/utils/rotulos'
import { Button } from '@/shared/components/ui/button'
import type { ProblemaRegra } from '@/shared/utils/erroApi'
import {
  CAMPOS,
  condicaoPadrao,
  NOMES_CAMPOS,
  type NomeCampo,
  type Operador,
  operadoresDo,
  ROTULO_OPERADOR,
  SINAIS,
} from '../utils/campos'
import { type CondicaoEditavel, novaChave, temProblema } from '../utils/politicaEditavel'
import { EntradaDecimal } from './EntradaDecimal'

interface EditorCondicoesProps {
  condicoes: CondicaoEditavel[]
  aoMudar: (condicoes: CondicaoEditavel[]) => void
  /** Caminho da regra no JSON ("faixas[1]"), para casar com os erros do servidor. */
  caminho: string
  problemas: ProblemaRegra[]
  /** Ex.: "o corte nega quando…" / "a faixa vale quando…". */
  legenda: string
}

export function EditorCondicoes({ condicoes, aoMudar, caminho, problemas, legenda }: EditorCondicoesProps) {
  function trocar(chave: string, mudanca: Partial<CondicaoEditavel>) {
    aoMudar(condicoes.map((c) => (c.chave === chave ? { ...c, ...mudanca } : c)))
  }

  function trocarCampo(chave: string, campo: NomeCampo) {
    aoMudar(condicoes.map((c) => (c.chave === chave ? { ...condicaoPadrao(campo), chave } : c)))
  }

  return (
    <div className="space-y-2">
      <p className="text-sm text-muted-foreground">
        {legenda}
        {condicoes.length > 1 && ' (todas precisam valer)'}
      </p>
      {condicoes.length === 0 && (
        <p className="rounded-xl bg-secondary px-3 py-2 text-sm text-muted-foreground">Nenhuma condição.</p>
      )}
      <ul className="space-y-2">
        {condicoes.map((condicao, j) => {
          const n = j + 1
          const tipo = CAMPOS[condicao.campo].tipo
          const comErro = temProblema(problemas, `${caminho}.condicoes[${j}]`)
          return (
            <li
              key={condicao.chave}
              aria-label={`Condição ${n}`}
              className="grid grid-cols-[1fr_1fr_auto] items-center gap-2 rounded-xl bg-secondary p-2 sm:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)_minmax(0,1fr)_auto]"
            >
              {j > 0 && (
                <span className="col-span-full px-1 text-xs font-semibold text-muted-foreground uppercase">
                  e
                </span>
              )}
              <Campo
                rotulo={`Campo da condição ${n}`}
                rotuloOculto
                className="order-1 col-span-2 sm:order-none sm:col-span-1"
              >
                {(props) => (
                  <select
                    {...props}
                    className={CLASSE_SELECT}
                    value={condicao.campo}
                    onChange={(e) => trocarCampo(condicao.chave, e.target.value as NomeCampo)}
                  >
                    {NOMES_CAMPOS.map((nome) => (
                      <option key={nome} value={nome}>
                        {CAMPOS[nome].rotulo}
                      </option>
                    ))}
                  </select>
                )}
              </Campo>
              <Campo rotulo={`Operador da condição ${n}`} rotuloOculto className="order-3 sm:order-none">
                {(props) => (
                  <select
                    {...props}
                    className={CLASSE_SELECT}
                    value={condicao.operador}
                    onChange={(e) => trocar(condicao.chave, { operador: e.target.value as Operador })}
                  >
                    {operadoresDo(condicao.campo).map((op) => (
                      <option key={op} value={op}>
                        {ROTULO_OPERADOR[op]}
                      </option>
                    ))}
                  </select>
                )}
              </Campo>
              <Campo rotulo={`Valor da condição ${n}`} rotuloOculto className="order-4 sm:order-none">
                {(props) =>
                  tipo === 'sinal' ? (
                    <select
                      {...props}
                      aria-invalid={comErro}
                      className={CLASSE_SELECT}
                      value={String(condicao.valor)}
                      onChange={(e) => trocar(condicao.chave, { valor: e.target.value })}
                    >
                      {SINAIS.map((sinal) => (
                        <option key={sinal} value={sinal}>
                          {ROTULO_SINAL[sinal]}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <div className="relative">
                      {tipo === 'dinheiro' && (
                        <span
                          aria-hidden="true"
                          className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-sm text-muted-foreground"
                        >
                          R$
                        </span>
                      )}
                      <EntradaDecimal
                        {...props}
                        aria-invalid={comErro}
                        className={tipo === 'dinheiro' ? 'pl-9' : undefined}
                        valor={Number(condicao.valor)}
                        casas={tipo === 'dinheiro' ? 2 : 0}
                        aoMudar={(valor) => trocar(condicao.chave, { valor })}
                      />
                    </div>
                  )
                }
              </Campo>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="order-2 justify-self-end sm:order-none"
                aria-label={`Remover condição ${n}`}
                onClick={() => aoMudar(condicoes.filter((c) => c.chave !== condicao.chave))}
              >
                <Trash2 aria-hidden="true" />
              </Button>
            </li>
          )
        })}
      </ul>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => aoMudar([...condicoes, { ...condicaoPadrao('score'), chave: novaChave() }])}
      >
        <Plus aria-hidden="true" />
        Adicionar condição
      </Button>
    </div>
  )
}
