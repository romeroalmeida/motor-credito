import type { Schemas } from '@/shared/types'

type Politica = Schemas['Politica']

/** Modelo "Empréstimo PF" do desenho §5, no formato que o painel edita. */
export function modeloEmprestimoPF(): Politica {
  return {
    cortes: [
      {
        codigo: 'FRAUDE_ALTA',
        texto: 'Risco de fraude alto',
        condicoes: [{ campo: 'fraude.risco', operador: '>=', valor: 70 }],
        cel: '',
      },
      {
        codigo: 'RESTRICOES',
        texto: 'Mais de duas restrições no bureau',
        condicoes: [{ campo: 'bureau.restricoes', operador: '>', valor: 2 }],
        cel: '',
      },
      {
        codigo: 'MENOR_IDADE',
        texto: 'Solicitante menor de idade',
        condicoes: [{ campo: 'idade', operador: '<', valor: 18 }],
        cel: '',
      },
    ],
    faixas: [
      {
        rotulo: 'C',
        condicoes: [{ campo: 'score', operador: '<=', valor: 300 }],
        cel: '',
        nega: true,
        taxa_bps: 0,
        limite_fixo: 0,
        multiplo_renda_x10: 0,
      },
      {
        rotulo: 'B',
        condicoes: [
          { campo: 'score', operador: '>', valor: 300 },
          { campo: 'score', operador: '<=', valor: 600 },
        ],
        cel: '',
        nega: false,
        taxa_bps: 590,
        limite_fixo: 300000,
        multiplo_renda_x10: 30,
      },
      {
        rotulo: 'A',
        condicoes: [{ campo: 'score', operador: '>', valor: 600 }],
        cel: '',
        nega: false,
        taxa_bps: 290,
        limite_fixo: 1500000,
        multiplo_renda_x10: 80,
      },
    ],
    contraproposta: { ativa: true, prazo_maximo: 0, acrescimo_taxa_bps: 0 },
    expressao_avancada: false,
  }
}
