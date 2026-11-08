import { documentoValido, mascararDocumento, normalizarDocumento } from './documento'

describe('documentoValido', () => {
  it.each([
    ['12ABC34501DE35', 'exemplo oficial da Receita (CNPJ alfanumérico)'],
    ['12.ABC.345/01DE-35', 'CNPJ alfanumérico com máscara'],
    ['12abc34501de35', 'minúsculas viram maiúsculas'],
    ['11222333000181', 'CNPJ numérico'],
    ['11.222.333/0001-81', 'CNPJ numérico com máscara'],
    ['52998224725', 'CPF'],
    ['529.982.247-25', 'CPF com máscara'],
  ])('aceita %s (%s)', (doc) => {
    expect(documentoValido(doc)).toBe(true)
  })

  it.each([
    ['12ABC34501DE36', 'DV errado no CNPJ alfanumérico'],
    ['12ABC34501DE3A', 'DV com letra'],
    ['11222333000182', 'DV errado no CNPJ numérico'],
    ['52998224726', 'DV errado no CPF'],
    ['5299822472A', 'CPF com letra'],
    ['11111111111', 'CPF repetido (DV bate)'],
    ['00000000000000', 'CNPJ repetido (DV bate)'],
    ['AAAAAAAAAAAAAA', 'CNPJ todo letra repetida'],
    ['5299822472', 'curto'],
    ['', 'vazio'],
    ['529_982_247_25', 'caractere fora da máscara'],
  ])('recusa %s (%s)', (doc) => {
    expect(documentoValido(doc)).toBe(false)
  })
})

describe('normalizarDocumento', () => {
  it('tira máscara e põe em maiúscula', () => {
    expect(normalizarDocumento(' 12.abc.345/01de-35 ')).toBe('12ABC34501DE35')
  })
})

describe('mascararDocumento', () => {
  it.each([
    ['529', '529'],
    ['5299', '529.9'],
    ['52998224725', '529.982.247-25'],
    ['112223330001', '11.222.333/0001'],
    ['11222333000181', '11.222.333/0001-81'],
    ['12abc', '12.ABC'],
    ['12ABC34501DE35999', '12.ABC.345/01DE-35'],
  ])('%s → %s', (entrada, esperado) => {
    expect(mascararDocumento(entrada)).toBe(esperado)
  })
})
