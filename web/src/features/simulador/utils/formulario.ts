import { lerDecimal } from '@/shared/utils/formato'
import type { RequisicaoConsulta } from '../services/simuladorService'

/** O que está digitado na tela, em texto (dinheiro em reais pt-BR; a conversão para centavos é em montarRequisicao). */
export interface Formulario {
  documento: string
  valor: string
  prazo: string
  nascimento: string
  renda: string
  nome: string
  email: string
  telefone: string
  ip: string
  deviceId: string
}

export const FORMULARIO_VAZIO: Formulario = {
  documento: '',
  valor: '10.000,00',
  prazo: '24',
  nascimento: '',
  renda: '',
  nome: '',
  email: '',
  telefone: '',
  ip: '',
  deviceId: '',
}

export interface Exemplo {
  rotulo: string
  campos: Partial<Formulario>
  aviso?: string
}

// Fixtures do bureau simulado (internal/bureau/README.md); o CNPJ e o CPF do Serpro são consultas reais.
export const EXEMPLOS: readonly Exemplo[] = [
  { rotulo: 'Aprovado', campos: { documento: '000.000.004-34', valor: '10.000,00', prazo: '24' } },
  { rotulo: 'Contraproposta', campos: { documento: '000.000.004-34', valor: '50.000,00', prazo: '24' } },
  {
    rotulo: 'Pré-aprovação',
    campos: { documento: '000.000.004-34', valor: '', prazo: '', renda: '5.000,00' },
    aviso: 'Sem valor e prazo: a API devolve o limite do cliente e as parcelas nos prazos da política.',
  },
  { rotulo: 'Negado', campos: { documento: '000.000.003-53', valor: '1.000,00', prazo: '12' } },
  { rotulo: 'Falecido', campos: { documento: '000.000.001-91', valor: '1.000,00', prazo: '12' } },
  { rotulo: 'CPF inválido', campos: { documento: '123.456.789-00', valor: '1.000,00', prazo: '12' } },
  { rotulo: 'CNPJ real', campos: { documento: '00.000.000/0001-91', valor: '100.000,00', prazo: '36' } },
  {
    rotulo: 'Serpro',
    campos: { documento: '404.428.201-35', nascimento: '1970-11-14', valor: '5.000,00', prazo: '12' },
    aviso:
      'Consulta real no ambiente de testes do Serpro (só quando a API sobe com BUREAU_CPF=serpro-trial). A cota do trial é pequena: use com moderação.',
  },
]

export type ErrosFormulario = Partial<Record<'documento' | 'valor' | 'prazo' | 'renda', string>>

const PRAZO_MAXIMO = 420

/**
 * Converte a tela no corpo da /v1/consultas. O documento vai como digitado, mesmo inválido: quem julga é a API
 * (o exemplo "CPF inválido" existe para mostrar a resposta real). Só barra o que não dá para montar.
 */
export function montarRequisicao(f: Formulario): { requisicao?: RequisicaoConsulta; erros: ErrosFormulario } {
  const valor = lerDecimal(f.valor, 2)
  const prazo = lerDecimal(f.prazo, 0)
  const renda = f.renda ? lerDecimal(f.renda, 2) : undefined
  // Valor e prazo vazios = consulta de limite (sem pedido); só um dos dois é erro no que faltou.
  const semPedido = !f.valor.trim() && !f.prazo.trim()
  const erros: ErrosFormulario = {
    documento: f.documento.trim() ? undefined : 'Informe um CPF ou CNPJ.',
    valor:
      semPedido || valor > 0 ? undefined : 'Valor em reais, ex.: 10.000,00 (ou deixe valor e prazo vazios)',
    prazo:
      semPedido || (prazo >= 1 && prazo <= PRAZO_MAXIMO)
        ? undefined
        : `De 1 a ${PRAZO_MAXIMO} meses (ou deixe valor e prazo vazios).`,
    renda: renda === undefined || Number.isFinite(renda) ? undefined : 'Renda em reais, ex.: 5.000,00',
  }
  if (Object.values(erros).some(Boolean)) return { erros }
  const dados = Object.fromEntries(
    Object.entries({
      nome: f.nome.trim(),
      nascimento: f.nascimento,
      renda,
      email: f.email.trim(),
      telefone: f.telefone.trim(),
      ip: f.ip.trim(),
      device_id: f.deviceId.trim(),
    }).filter(([, v]) => v !== '' && v !== undefined),
  )
  return {
    erros,
    requisicao: {
      documento: f.documento,
      finalidade: 'concessao_credito',
      ...(!semPedido && { pedido: { valor, prazo } }),
      ...(Object.keys(dados).length > 0 && { dados }),
    },
  }
}
