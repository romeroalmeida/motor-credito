import { Check, Copy } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/shared/components/ui/button'

interface BotaoCopiarProps {
  texto: string
  /** Nome acessível: "Copiar corpo", "Copiar código curl"… */
  rotulo: string
}

export function BotaoCopiar({ texto, rotulo }: BotaoCopiarProps) {
  const [estado, setEstado] = useState<'parado' | 'copiado' | 'falhou'>('parado')

  async function copiar() {
    try {
      await navigator.clipboard.writeText(texto)
      setEstado('copiado')
    } catch {
      setEstado('falhou') // sem permissão: o bloco é selecionável para copiar à mão
    }
  }

  return (
    <span className="inline-flex items-center gap-2">
      <Button variant="outline" size="sm" onClick={copiar} aria-label={rotulo}>
        {estado === 'copiado' ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}
        {estado === 'copiado' ? 'Copiado' : 'Copiar'}
      </Button>
      <span aria-live="polite" className="text-xs text-muted-foreground">
        {estado === 'falhou' && 'Selecione e copie à mão.'}
      </span>
    </span>
  )
}

/** Código com rolagem horizontal só no bloco: a página não estica no celular. */
export function BlocoCodigo({ codigo, rotulo }: { codigo: string; rotulo: string }) {
  return (
    <pre
      tabIndex={0}
      aria-label={rotulo}
      className="max-h-[28rem] overflow-auto rounded-xl bg-primary p-4 font-mono text-xs leading-relaxed text-primary-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
    >
      <code>{codigo}</code>
    </pre>
  )
}
