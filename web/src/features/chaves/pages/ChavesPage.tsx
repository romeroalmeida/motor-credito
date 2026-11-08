import { KeyRound, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import {
  AvisoErro,
  CabecalhoPagina,
  Campo,
  Carregando,
  Cartao,
  ConfirmarAcao,
  ErroCarregar,
  Etiqueta,
  TituloCartao,
  Vazio,
} from '@/shared/components'
import { Button } from '@/shared/components/ui/button'
import { Input } from '@/shared/components/ui/input'
import { lerErroApi } from '@/shared/utils/erroApi'
import { formatarDataHora } from '@/shared/utils/formato'
import { DialogoChaveCriada } from '../components/DialogoChaveCriada'
import { IpsPermitidos } from '../components/IpsPermitidos'
import { useChaves, useCriarChave, useGirarChave, useRevogarChave } from '../hooks/useChaves'
import type { Chave, ChaveCriada } from '../services/chavesService'

type Acao = { tipo: 'girar' | 'revogar'; chave: Chave }

export function ChavesPage() {
  const chaves = useChaves()
  const criar = useCriarChave()
  const girar = useGirarChave()
  const revogar = useRevogarChave()
  const [nome, setNome] = useState('')
  const [criada, setCriada] = useState<ChaveCriada | null>(null)
  const [acao, setAcao] = useState<Acao | null>(null)

  function esquecerSegredo() {
    setCriada(null)
    criar.reset()
    girar.reset()
  }

  function criarChave(evento: FormEvent) {
    evento.preventDefault()
    criar.mutate(nome.trim(), {
      onSuccess: (nova) => {
        setNome('')
        setCriada(nova)
      },
    })
  }

  function confirmarAcao() {
    if (!acao) return
    if (acao.tipo === 'revogar') {
      revogar.mutate(acao.chave.id, { onSuccess: () => setAcao(null) })
      return
    }
    girar.mutate(acao.chave.id, {
      onSuccess: (nova) => {
        setAcao(null)
        setCriada(nova)
      },
    })
  }

  const mutacaoDaAcao = acao?.tipo === 'revogar' ? revogar : girar

  return (
    <>
      <CabecalhoPagina
        titulo="Chaves de API"
        descricao="Cada chave tem um id (vai no Bearer) e um segredo (assina as chamadas a POST /v1/consultas). O segredo aparece uma única vez, na criação."
      />
      <Cartao aria-labelledby="titulo-chaves">
        <TituloCartao id="titulo-chaves">Chaves</TituloCartao>
        <form onSubmit={criarChave} className="mb-5 flex flex-wrap items-end gap-2">
          <Campo rotulo="Nome da nova chave" className="min-w-48 flex-1">
            {(props) => (
              <Input
                {...props}
                maxLength={80}
                placeholder="Ex.: produção"
                value={nome}
                onChange={(e) => setNome(e.target.value)}
              />
            )}
          </Campo>
          <Button type="submit" disabled={criar.isPending}>
            <Plus aria-hidden="true" />
            {criar.isPending ? 'Criando…' : 'Criar chave'}
          </Button>
        </form>
        {criar.isError && <AvisoErro className="mb-4">{lerErroApi(criar.error).mensagem}</AvisoErro>}
        {chaves.isPending ? (
          <Carregando linhas={3} />
        ) : chaves.isError ? (
          <ErroCarregar
            mensagem={lerErroApi(chaves.error).mensagem}
            aoTentarDeNovo={() => chaves.refetch()}
          />
        ) : chaves.data.length === 0 ? (
          <Vazio icone={KeyRound} titulo="Nenhuma chave ainda">
            Crie a primeira para sua integração começar a consultar.
          </Vazio>
        ) : (
          <ul className="divide-y divide-border">
            {chaves.data.map((chave) => (
              <ItemChave key={chave.id} chave={chave} aoAgir={(tipo) => setAcao({ tipo, chave })} />
            ))}
          </ul>
        )}
      </Cartao>

      <IpsPermitidos />

      <DialogoChaveCriada chave={criada} aoFechar={esquecerSegredo} />

      <ConfirmarAcao
        aberto={acao !== null}
        aoMudarAberto={(aberto) => {
          if (!aberto) {
            setAcao(null)
            revogar.reset()
            girar.reset()
          }
        }}
        titulo={
          acao?.tipo === 'revogar'
            ? `Revogar a chave "${acao.chave.nome}"?`
            : `Girar a chave "${acao?.chave.nome}"?`
        }
        descricao={
          acao?.tipo === 'revogar'
            ? 'Chamadas com esta chave passam a ser recusadas na hora. Não dá para desfazer.'
            : 'Criamos uma chave nova com o mesmo nome e revogamos esta na hora. Atualize a integração com o novo segredo antes de seguir.'
        }
        rotuloConfirmar={acao?.tipo === 'revogar' ? 'Revogar chave' : 'Girar chave'}
        destrutiva
        carregando={mutacaoDaAcao.isPending}
        aoConfirmar={confirmarAcao}
      >
        {mutacaoDaAcao.isError && <AvisoErro>{lerErroApi(mutacaoDaAcao.error).mensagem}</AvisoErro>}
      </ConfirmarAcao>
    </>
  )
}

function ItemChave({ chave, aoAgir }: { chave: Chave; aoAgir: (tipo: Acao['tipo']) => void }) {
  const revogada = chave.revogada_em !== null
  return (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-2 py-3">
      <div className="min-w-0 flex-1 space-y-1">
        <p className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{chave.nome || 'Sem nome'}</span>
          {revogada ? <Etiqueta>Revogada</Etiqueta> : <Etiqueta tom="aprovado">Ativa</Etiqueta>}
        </p>
        <p className="font-mono text-xs break-all text-muted-foreground">{chave.prefixo}</p>
        <p className="text-xs text-muted-foreground tabular-nums">
          Criada em {formatarDataHora(chave.criada_em)}
          {revogada && chave.revogada_em
            ? ` · revogada em ${formatarDataHora(chave.revogada_em)}`
            : ` · ${chave.ultimo_uso_em ? `último uso em ${formatarDataHora(chave.ultimo_uso_em)}` : 'nunca usada'}`}
        </p>
      </div>
      {!revogada && (
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => aoAgir('girar')}>
            <RefreshCw aria-hidden="true" />
            Girar
          </Button>
          <Button variant="outline" size="sm" onClick={() => aoAgir('revogar')}>
            <Trash2 aria-hidden="true" />
            Revogar
          </Button>
        </div>
      )}
    </li>
  )
}
