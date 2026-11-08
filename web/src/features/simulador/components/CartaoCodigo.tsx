import { TriangleAlert } from 'lucide-react'
import { Cartao, TituloCartao } from '@/shared/components'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/shared/components/ui/tabs'
import type { ResultadoSimulador } from '../services/simuladorService'
import { BlocoCodigo, BotaoCopiar } from './BotaoCopiar'

const LINGUAGENS = [
  { chave: 'curl', rotulo: 'curl' },
  { chave: 'node', rotulo: 'Node' },
  { chave: 'python', rotulo: 'Python' },
] as const

export function CartaoCodigo({ trechos }: { trechos: ResultadoSimulador['trechos'] }) {
  return (
    <Cartao aria-labelledby="titulo-codigo">
      <TituloCartao id="titulo-codigo" descricao="A mesma requisição, para copiar para o seu sistema.">
        Código
      </TituloCartao>
      <p className="mb-4 flex items-start gap-2 rounded-xl bg-secondary p-3 text-sm">
        <TriangleAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
        <span>
          Defina <code className="font-mono text-xs">MOTOR_SEGREDO</code> com o segredo mostrado na criação da
          chave. O código calcula a assinatura e gera timestamp e Idempotency-Key novos a cada execução.
        </span>
      </p>
      <Tabs defaultValue="curl">
        <TabsList aria-label="Linguagem">
          {LINGUAGENS.map((l) => (
            <TabsTrigger key={l.chave} value={l.chave}>
              {l.rotulo}
            </TabsTrigger>
          ))}
        </TabsList>
        {LINGUAGENS.map((l) => (
          <TabsContent key={l.chave} value={l.chave} className="space-y-2">
            <div className="flex justify-end">
              <BotaoCopiar texto={trechos[l.chave]} rotulo={`Copiar código ${l.rotulo}`} />
            </div>
            <BlocoCodigo codigo={trechos[l.chave]} rotulo={`Código ${l.rotulo}`} />
          </TabsContent>
        ))}
      </Tabs>
    </Cartao>
  )
}
