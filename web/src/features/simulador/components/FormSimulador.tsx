import { RefreshCw, Repeat, Send } from 'lucide-react'
import { type ChangeEvent, type FormEvent, useState } from 'react'
import { Campo, Cartao, CLASSE_SELECT, TituloCartao } from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { mascararDocumento } from '@/shared/utils/documento'
import type { ChaveAtiva, EntradaSimulador } from '../services/simuladorService'
import {
  EXEMPLOS,
  type ErrosFormulario,
  type Formulario,
  FORMULARIO_VAZIO,
  montarRequisicao,
} from '../utils/formulario'

interface FormSimuladorProps {
  chaves: ChaveAtiva[]
  enviando: boolean
  /** Idempotency-Key da última requisição enviada, para demonstrar a idempotência. */
  ultimaIdem: string | null
  aoEnviar: (entrada: EntradaSimulador) => void
}

export function FormSimulador({ chaves, enviando, ultimaIdem, aoEnviar }: FormSimuladorProps) {
  const [chaveId, setChaveId] = useState(chaves[0]?.id ?? '')
  const [form, setForm] = useState<Formulario>(FORMULARIO_VAZIO)
  const [idem, setIdem] = useState('')
  const [aviso, setAviso] = useState<string | undefined>()
  const [erros, setErros] = useState<ErrosFormulario>({})

  const campo = (nome: keyof Formulario) => ({
    value: form[nome],
    onChange: (e: ChangeEvent<HTMLInputElement>) => setForm((f) => ({ ...f, [nome]: e.target.value })),
  })

  function enviar(evento: FormEvent) {
    evento.preventDefault()
    const { requisicao, erros: novos } = montarRequisicao(form)
    setErros(novos)
    if (requisicao) aoEnviar({ chave_id: chaveId, idempotency_key: idem.trim() || undefined, requisicao })
  }

  return (
    <Cartao aria-labelledby="titulo-form-simulador">
      <TituloCartao
        id="titulo-form-simulador"
        descricao="Faz uma chamada real da POST /v1/consultas com a chave escolhida. A consulta é gravada e cobrada como qualquer outra."
      >
        Montar a consulta
      </TituloCartao>

      <div className="mb-5 space-y-2">
        <p id="rotulo-exemplos" className="text-sm font-medium">
          Exemplos
        </p>
        <div role="group" aria-labelledby="rotulo-exemplos" className="flex flex-wrap gap-2">
          {EXEMPLOS.map((exemplo) => (
            <Button
              key={exemplo.rotulo}
              type="button"
              variant="secondary"
              size="sm"
              onClick={() => {
                setForm({ ...FORMULARIO_VAZIO, ...exemplo.campos })
                setAviso(exemplo.aviso)
                setErros({})
              }}
            >
              {exemplo.rotulo}
            </Button>
          ))}
        </div>
        {aviso && (
          <p role="status" className="rounded-xl bg-secondary p-3 text-xs">
            {aviso}
          </p>
        )}
      </div>

      <form onSubmit={enviar} noValidate className="grid gap-3 sm:grid-cols-2">
        <Campo
          rotulo="Chave de API"
          className="sm:col-span-2"
          ajuda="Só as ativas. O segredo fica no servidor."
        >
          {(props) => (
            <select
              {...props}
              className={CLASSE_SELECT}
              value={chaveId}
              onChange={(e) => setChaveId(e.target.value)}
            >
              {chaves.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.nome ? `${c.nome} · ${c.prefixo}` : c.prefixo}
                </option>
              ))}
            </select>
          )}
        </Campo>
        <Campo rotulo="CPF ou CNPJ" erro={erros.documento} className="sm:col-span-2">
          {(props) => (
            <Input
              {...props}
              value={form.documento}
              autoComplete="off"
              spellCheck={false}
              onChange={(e) => setForm((f) => ({ ...f, documento: mascararDocumento(e.target.value) }))}
            />
          )}
        </Campo>
        <Campo
          rotulo="Valor pedido (R$)"
          ajuda="Vazio com o prazo vazio: consulta só o limite."
          erro={erros.valor}
        >
          {(props) => <Input {...props} inputMode="decimal" {...campo('valor')} />}
        </Campo>
        <Campo rotulo="Prazo (meses)" erro={erros.prazo}>
          {(props) => <Input {...props} inputMode="numeric" {...campo('prazo')} />}
        </Campo>
        <Campo rotulo="Nascimento" ajuda="Com o Serpro ligado, sem ele o CPF fica não verificado.">
          {(props) => <Input {...props} type="date" {...campo('nascimento')} />}
        </Campo>
        <Campo rotulo="Renda mensal (R$)" erro={erros.renda}>
          {(props) => <Input {...props} inputMode="decimal" {...campo('renda')} />}
        </Campo>

        <details className="group rounded-xl bg-secondary/60 p-3 sm:col-span-2">
          <summary className="cursor-pointer text-sm font-medium select-none">Dados opcionais</summary>
          <p className="mt-1 text-xs text-muted-foreground">
            Cada dado enviado liga mais checagens do antifraude.
          </p>
          <div className="mt-3 grid gap-3 sm:grid-cols-2">
            <Campo rotulo="Nome">
              {(props) => <Input {...props} autoComplete="off" {...campo('nome')} />}
            </Campo>
            <Campo rotulo="E-mail">
              {(props) => <Input {...props} type="email" autoComplete="off" {...campo('email')} />}
            </Campo>
            <Campo rotulo="Telefone">
              {(props) => <Input {...props} type="tel" autoComplete="off" {...campo('telefone')} />}
            </Campo>
            <Campo rotulo="IP do solicitante">
              {(props) => <Input {...props} spellCheck={false} {...campo('ip')} />}
            </Campo>
            <Campo rotulo="Device ID" className="sm:col-span-2">
              {(props) => <Input {...props} spellCheck={false} {...campo('deviceId')} />}
            </Campo>
          </div>
        </details>

        <Campo
          rotulo="Idempotency-Key"
          className="sm:col-span-2"
          ajuda="Em branco, o servidor gera uma nova a cada envio. Reusar a última com o mesmo corpo devolve a mesma decisão."
        >
          {(props) => (
            <div className="flex flex-wrap gap-2">
              <Input
                {...props}
                value={idem}
                spellCheck={false}
                autoComplete="off"
                className="min-w-0 flex-1 font-mono"
                onChange={(e) => setIdem(e.target.value)}
              />
              <Button type="button" variant="outline" onClick={() => setIdem(crypto.randomUUID())}>
                <RefreshCw aria-hidden="true" />
                Nova
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={!ultimaIdem}
                onClick={() => ultimaIdem && setIdem(ultimaIdem)}
              >
                <Repeat aria-hidden="true" />
                Reusar a última
              </Button>
            </div>
          )}
        </Campo>

        <Button type="submit" className="sm:col-span-2" disabled={enviando || !chaveId}>
          <Send aria-hidden="true" />
          {enviando ? 'Enviando…' : 'Enviar consulta'}
        </Button>
      </form>
    </Cartao>
  )
}
