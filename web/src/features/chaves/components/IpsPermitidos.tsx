import { type FormEvent, useState } from 'react'
import { AvisoErro, Campo, Carregando, Cartao, ErroCarregar, TituloCartao } from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { Textarea } from '@/shared/components/ui/textarea'
import { lerErroApi } from '@/shared/utils/erroApi'
import { useIps, useSalvarIps } from '../hooks/useChaves'

const MAXIMO_IPS = 50
// Filtro de digitação: o servidor é quem valida de verdade (netip.ParsePrefix).
const IPV4 = /^(\d{1,3}\.){3}\d{1,3}(\/\d{1,2})?$/
const IPV6 = /^[0-9a-f:]*:[0-9a-f:]*(\/\d{1,3})?$/i

function linhasDeIp(texto: string): string[] {
  return texto
    .split('\n')
    .map((linha) => linha.trim())
    .filter(Boolean)
}

export function IpsPermitidos() {
  const ips = useIps()

  return (
    <Cartao aria-labelledby="titulo-ips">
      <TituloCartao
        id="titulo-ips"
        descricao="Só estes endereços podem chamar a API com as suas chaves. Lista vazia aceita qualquer IP."
      >
        IPs permitidos
      </TituloCartao>
      {ips.isPending ? (
        <Carregando linhas={2} />
      ) : ips.isError ? (
        <ErroCarregar mensagem={lerErroApi(ips.error).mensagem} aoTentarDeNovo={() => ips.refetch()} />
      ) : (
        <FormIps inicial={ips.data} />
      )}
    </Cartao>
  )
}

function FormIps({ inicial }: { inicial: string[] }) {
  const salvar = useSalvarIps()
  const [texto, setTexto] = useState(inicial.join('\n'))
  const [erro, setErro] = useState<string>()
  const [salvo, setSalvo] = useState(false)

  function enviar(evento: FormEvent) {
    evento.preventDefault()
    const lista = linhasDeIp(texto)
    const invalidos = lista.filter((ip) => !IPV4.test(ip) && !IPV6.test(ip))
    if (invalidos.length > 0) return setErro(`Não parece IP ou faixa CIDR: ${invalidos.join(', ')}`)
    if (lista.length > MAXIMO_IPS) return setErro(`No máximo ${MAXIMO_IPS} linhas.`)
    setErro(undefined)
    salvar.mutate(lista, { onSuccess: () => setSalvo(true) })
  }

  const erroServidor = salvar.isError ? lerErroApi(salvar.error) : null

  return (
    <form onSubmit={enviar} noValidate className="space-y-3">
      <Campo rotulo="Um IP ou faixa por linha" ajuda="Ex.: 200.150.10.1 ou 200.150.10.0/24" erro={erro}>
        {(props) => (
          <Textarea
            {...props}
            className="min-h-28 font-mono text-sm md:text-sm"
            spellCheck={false}
            value={texto}
            onChange={(e) => {
              setTexto(e.target.value)
              setSalvo(false)
            }}
          />
        )}
      </Campo>
      {erroServidor && (
        <AvisoErro>
          {erroServidor.problemas.length > 0
            ? erroServidor.problemas.map((p) => `${p.caminho}: ${p.motivo}`).join(' · ')
            : erroServidor.mensagem}
        </AvisoErro>
      )}
      <div className="flex items-center gap-3">
        <Button type="submit" variant="outline" disabled={salvar.isPending}>
          {salvar.isPending ? 'Salvando…' : 'Salvar IPs'}
        </Button>
        {salvo && (
          <span role="status" className="text-sm text-muted-foreground">
            Lista salva.
          </span>
        )}
      </div>
    </form>
  )
}
