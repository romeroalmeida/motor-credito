import { Search, X } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { Campo, CLASSE_SELECT } from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import type { StatusDecisao } from '@/shared/types'
import { documentoValido, mascararDocumento, normalizarDocumento } from '@/shared/utils/documento'
import type { FiltrosConsultas as Filtros } from '../services/consultasService'

const OPCOES_STATUS: { valor: StatusDecisao; rotulo: string }[] = [
  { valor: 'aprovado', rotulo: 'Aprovado' },
  { valor: 'contraproposta', rotulo: 'Contraproposta' },
  { valor: 'pre_aprovado', rotulo: 'Pré-aprovado' },
  { valor: 'negado', rotulo: 'Negado' },
  { valor: 'documento_invalido', rotulo: 'Documento inválido' },
]

const FAIXAS = ['A', 'B', 'C', 'D']

interface FiltrosConsultasProps {
  filtros: Filtros
  aoMudar: (filtros: Filtros) => void
}

export function FiltrosConsultas({ filtros, aoMudar }: FiltrosConsultasProps) {
  const [documento, setDocumento] = useState(filtros.documento ? mascararDocumento(filtros.documento) : '')
  const [erroDocumento, setErroDocumento] = useState<string>()
  const temFiltro = Object.values(filtros).some(Boolean)

  function trocar<K extends keyof Filtros>(campo: K, valor: Filtros[K] | '') {
    aoMudar({ ...filtros, [campo]: valor || undefined })
  }

  function buscarDocumento(evento: FormEvent) {
    evento.preventDefault()
    if (documento === '') {
      setErroDocumento(undefined)
      trocar('documento', undefined)
      return
    }
    if (!documentoValido(documento)) {
      setErroDocumento('Digite o CPF ou CNPJ completo. A busca não aceita parte do número.')
      return
    }
    setErroDocumento(undefined)
    trocar('documento', normalizarDocumento(documento))
  }

  function limpar() {
    setDocumento('')
    setErroDocumento(undefined)
    aoMudar({})
  }

  return (
    <div className="space-y-4">
      <form
        onSubmit={buscarDocumento}
        role="search"
        aria-label="Buscar por documento"
        className="flex items-start gap-2"
      >
        <Campo rotulo="CPF ou CNPJ" rotuloOculto erro={erroDocumento} className="flex-1">
          {(props) => (
            <Input
              {...props}
              value={documento}
              onChange={(e) => setDocumento(mascararDocumento(e.target.value))}
              placeholder="CPF ou CNPJ completo"
              autoComplete="off"
              spellCheck={false}
            />
          )}
        </Campo>
        <Button type="submit" variant="default">
          <Search aria-hidden="true" />
          Buscar
        </Button>
      </form>
      <div className="grid grid-cols-2 gap-3 md:grid-cols-[repeat(4,minmax(0,1fr))_auto] md:items-end">
        <Campo rotulo="Status">
          {(props) => (
            <select
              {...props}
              className={CLASSE_SELECT}
              value={filtros.status ?? ''}
              onChange={(e) => trocar('status', e.target.value as StatusDecisao | '')}
            >
              <option value="">Todos</option>
              {OPCOES_STATUS.map(({ valor, rotulo }) => (
                <option key={valor} value={valor}>
                  {rotulo}
                </option>
              ))}
            </select>
          )}
        </Campo>
        <Campo rotulo="Faixa de risco">
          {(props) => (
            <select
              {...props}
              className={CLASSE_SELECT}
              value={filtros.faixa ?? ''}
              onChange={(e) => trocar('faixa', e.target.value)}
            >
              <option value="">Todas</option>
              {FAIXAS.map((faixa) => (
                <option key={faixa} value={faixa}>
                  {faixa}
                </option>
              ))}
            </select>
          )}
        </Campo>
        <Campo rotulo="De">
          {(props) => (
            <Input
              {...props}
              type="date"
              value={filtros.desde ?? ''}
              max={filtros.ate}
              onChange={(e) => trocar('desde', e.target.value)}
            />
          )}
        </Campo>
        <Campo rotulo="Até">
          {(props) => (
            <Input
              {...props}
              type="date"
              value={filtros.ate ?? ''}
              min={filtros.desde}
              onChange={(e) => trocar('ate', e.target.value)}
            />
          )}
        </Campo>
        {temFiltro && (
          <Button variant="ghost" onClick={limpar} className="col-span-2 md:col-span-1">
            <X aria-hidden="true" />
            Limpar filtros
          </Button>
        )}
      </div>
    </div>
  )
}
