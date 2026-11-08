import type { Schemas } from '@/shared/types'

type Sinal = Schemas['Antifraude']['sinais'][number]

export const ROTULO_SINAL: Record<Sinal, string> = {
  cpf_irregular: 'CPF irregular',
  cnpj_inativo: 'CNPJ inativo',
  velocidade_alta: 'Muitas consultas em 24 h',
  nome_divergente: 'Nome divergente',
  nascimento_divergente: 'Nascimento divergente',
  email_descartavel: 'E-mail descartável',
  ip_estrangeiro: 'IP estrangeiro',
  ip_datacenter: 'IP de datacenter',
  dispositivo_multiplos_cpfs: 'Dispositivo em vários documentos',
}

/** Visão geral devolve o sinal como texto livre: sinal desconhecido aparece cru, sem quebrar a tela. */
export function rotuloSinal(sinal: string): string {
  return ROTULO_SINAL[sinal as Sinal] ?? sinal
}
