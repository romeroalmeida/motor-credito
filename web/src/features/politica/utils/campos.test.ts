import casosDoContrato from '../../../../../api/cel-do-painel.json'
import { type Condicao, condicoesParaCel } from './campos'

// Mesmo arquivo que internal/politica/painel_test.go compila no servidor: se o editor mudar a string, um dos dois quebra.
const casos = casosDoContrato as { condicoes: Condicao[]; cel: string }[]

describe('CEL gerado pelo editor visual', () => {
  it.each(casos)('$cel', ({ condicoes, cel }) => {
    expect(condicoesParaCel(condicoes)).toBe(cel)
  })
})
