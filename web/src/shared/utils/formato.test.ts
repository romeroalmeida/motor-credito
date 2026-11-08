import {
  escreverDecimal,
  formatarCentavos,
  formatarMultiploX10,
  formatarPercentualBps,
  formatarTaxaBps,
  lerDecimal,
} from './formato'

// Intl usa espaço não separável entre "R$" e o número.
const semNbsp = (texto: string) => texto.replace(/\u00a0/g, ' ')

describe('formatarCentavos', () => {
  it.each([
    [0, 'R$ 0,00'],
    [1, 'R$ 0,01'],
    [99, 'R$ 0,99'],
    [123456, 'R$ 1.234,56'],
    [1000000, 'R$ 10.000,00'],
    [33742, 'R$ 337,42'],
  ])('%i centavos → %s', (centavos, esperado) => {
    expect(semNbsp(formatarCentavos(centavos))).toBe(esperado)
  })
})

describe('taxas e percentuais', () => {
  it.each([
    [290, '2,90% a.m.'],
    [590, '5,90% a.m.'],
    [5, '0,05% a.m.'],
    [0, '0,00% a.m.'],
  ])('taxa %i bps → %s', (bps, esperado) => {
    expect(formatarTaxaBps(bps)).toBe(esperado)
  })

  it('aprovação em bps vira percentual com uma casa', () => {
    expect(formatarPercentualBps(6500)).toBe('65,0%')
    expect(formatarPercentualBps(6667)).toBe('66,7%')
  })

  it('múltiplo da renda ×10', () => {
    expect(formatarMultiploX10(30)).toBe('3,0×')
    expect(formatarMultiploX10(85)).toBe('8,5×')
  })
})

describe('lerDecimal', () => {
  it.each([
    ['1.234,56', 2, 123456],
    ['1234,56', 2, 123456],
    ['R$ 1.234,5', 2, 123450],
    ['1500', 2, 150000],
    ['0,01', 2, 1],
    ['2,9', 2, 290],
    ['3', 1, 30],
    ['3,5', 1, 35],
    ['700', 0, 700],
  ])('"%s" com %i casas → %i', (texto, casas, esperado) => {
    expect(lerDecimal(texto, casas)).toBe(esperado)
  })

  it.each([[''], ['abc'], ['1,234'], ['1.23'], ['-5'], ['1,2,3'], ['12.34,5']])(
    '"%s" é inválido',
    (texto) => {
      expect(lerDecimal(texto, 2)).toBeNaN()
    },
  )

  it('número com decimais quando casas = 0 é inválido', () => {
    expect(lerDecimal('7,5', 0)).toBeNaN()
  })
})

describe('escreverDecimal', () => {
  it('é o inverso de lerDecimal', () => {
    for (const valor of [0, 1, 290, 123456, 100000000]) {
      expect(lerDecimal(escreverDecimal(valor, 2), 2)).toBe(valor)
    }
    expect(escreverDecimal(290, 2)).toBe('2,90')
    expect(escreverDecimal(30, 1)).toBe('3,0')
    expect(escreverDecimal(700, 0)).toBe('700')
    expect(escreverDecimal(Number.NaN, 2)).toBe('')
  })
})
