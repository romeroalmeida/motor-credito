import { FlaskConical } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import {
  AvisoErro,
  Campo,
  Cartao,
  FontesDados,
  ResumoAntifraude,
  ResumoDecisao,
  TituloCartao,
  TrilhaRegras,
} from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { lerErroApi, type ProblemaRegra } from '@/shared/utils/erroApi'
import { documentoValido, mascararDocumento } from '@/shared/utils/documento'
import { lerDecimal } from '@/shared/utils/formato'
import { useTestarPolitica } from '../hooks/usePolitica'
import type { Politica } from '../utils/politicaEditavel'

interface PainelTesteProps {
  /** Monta a política atual do editor; null quando há problema local (o editor já marcou). */
  obterPolitica: () => Politica | null
  aoRecusar: (problemas: ProblemaRegra[]) => void
}

interface Erros {
  documento?: string
  valor?: string
  prazo?: string
  renda?: string
}

const PRAZO_MAXIMO = 420

/** Roda o rascunho do editor (mesmo sem salvar) contra um documento de exemplo. Não grava consulta. */
export function PainelTeste({ obterPolitica, aoRecusar }: PainelTesteProps) {
  const testar = useTestarPolitica()
  const [documento, setDocumento] = useState('')
  const [valor, setValor] = useState('10.000,00')
  const [prazo, setPrazo] = useState('24')
  const [renda, setRenda] = useState('')
  const [nascimento, setNascimento] = useState('')
  const [erros, setErros] = useState<Erros>({})

  function enviar(evento: FormEvent) {
    evento.preventDefault()
    const centavos = lerDecimal(valor, 2)
    const meses = lerDecimal(prazo, 0)
    const rendaCentavos = renda ? lerDecimal(renda, 2) : null
    const novos: Erros = {
      documento: documentoValido(documento) ? undefined : 'CPF ou CNPJ com dígitos válidos.',
      valor: centavos > 0 ? undefined : 'Valor em reais, ex.: 10.000,00',
      prazo: meses >= 1 && meses <= PRAZO_MAXIMO ? undefined : `De 1 a ${PRAZO_MAXIMO} meses.`,
      renda:
        rendaCentavos === null || Number.isFinite(rendaCentavos)
          ? undefined
          : 'Renda em reais, ex.: 5.000,00',
    }
    setErros(novos)
    if (Object.values(novos).some(Boolean)) return
    const politica = obterPolitica()
    if (!politica) return
    testar.mutate(
      {
        documento,
        pedido: { valor: centavos, prazo: meses },
        dados:
          rendaCentavos === null && !nascimento
            ? undefined
            : { renda: rendaCentavos ?? undefined, nascimento: nascimento || undefined },
        politica,
      },
      {
        onError: (erro) => {
          const { status, problemas } = lerErroApi(erro)
          if (status === 422 || status === 400) aoRecusar(problemas)
        },
      },
    )
  }

  const erroGeral = testar.isError ? lerErroApi(testar.error) : null

  return (
    <Cartao aria-labelledby="titulo-teste">
      <TituloCartao
        id="titulo-teste"
        descricao="Usa o que está no editor agora, mesmo sem salvar. Não grava consulta."
      >
        Testar com um documento
      </TituloCartao>
      <form onSubmit={enviar} noValidate className="grid gap-3 sm:grid-cols-2">
        <Campo rotulo="CPF ou CNPJ" erro={erros.documento} className="sm:col-span-2">
          {(props) => (
            <Input
              {...props}
              value={documento}
              autoComplete="off"
              spellCheck={false}
              onChange={(e) => setDocumento(mascararDocumento(e.target.value))}
            />
          )}
        </Campo>
        <Campo rotulo="Valor pedido (R$)" erro={erros.valor}>
          {(props) => (
            <Input {...props} inputMode="decimal" value={valor} onChange={(e) => setValor(e.target.value)} />
          )}
        </Campo>
        <Campo rotulo="Prazo (meses)" erro={erros.prazo}>
          {(props) => (
            <Input {...props} inputMode="numeric" value={prazo} onChange={(e) => setPrazo(e.target.value)} />
          )}
        </Campo>
        <Campo rotulo="Renda mensal (R$)" ajuda="Opcional. Sem renda, vale o limite fixo." erro={erros.renda}>
          {(props) => (
            <Input {...props} inputMode="decimal" value={renda} onChange={(e) => setRenda(e.target.value)} />
          )}
        </Campo>
        <Campo rotulo="Nascimento" ajuda="Opcional. Com o Serpro ligado, sem ele o CPF fica não verificado.">
          {(props) => (
            <Input
              {...props}
              type="date"
              value={nascimento}
              onChange={(e) => setNascimento(e.target.value)}
            />
          )}
        </Campo>
        <Button type="submit" className="sm:col-span-2" disabled={testar.isPending}>
          <FlaskConical aria-hidden="true" />
          {testar.isPending ? 'Testando…' : 'Testar rascunho'}
        </Button>
      </form>
      {erroGeral && erroGeral.status !== 422 && <AvisoErro className="mt-4">{erroGeral.mensagem}</AvisoErro>}
      {erroGeral?.status === 422 && (
        <AvisoErro className="mt-4">
          A política não compila. Os problemas estão marcados nas regras.
        </AvisoErro>
      )}
      {testar.data && (
        <section aria-label="Resultado do teste" aria-live="polite" className="mt-6 space-y-6 border-t pt-6">
          <ResumoDecisao
            status={testar.data.status}
            score={testar.data.score}
            faixaRisco={testar.data.faixa_risco}
            oferta={testar.data.oferta}
            motivos={testar.data.motivos}
          />
          <div className="space-y-2">
            <h3 className="text-sm font-semibold">De onde vêm os dados</h3>
            <FontesDados
              fontes={testar.data.fontes}
              fatores={testar.data.score_fatores}
              cadastro={testar.data.cadastro}
            />
          </div>
          <div className="space-y-2">
            <h3 className="text-sm font-semibold">Regras avaliadas</h3>
            <TrilhaRegras trilha={testar.data.trilha} />
          </div>
          <div className="space-y-2">
            <h3 className="text-sm font-semibold">Antifraude</h3>
            <ResumoAntifraude antifraude={testar.data.antifraude} />
          </div>
        </section>
      )}
    </Cartao>
  )
}
