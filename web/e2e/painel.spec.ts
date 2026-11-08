import { expect, test } from '@playwright/test'
import { createHmac } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import path from 'node:path'

const RAIZ = path.resolve(import.meta.dirname, '../..')

interface Demo {
  email: string
  senha: string
  segredoTotp: string
  chaveApi: string
  segredoApi: string
}

/** O mesmo banco da API do compose, pela porta publicada no host. */
function urlBanco(): string {
  if (process.env.DATABASE_URL) return process.env.DATABASE_URL
  const senha = process.env.POSTGRES_APP_PASSWORD
  if (!senha) throw new Error('defina DATABASE_URL ou POSTGRES_APP_PASSWORD (make e2e lê o .env)')
  return `postgres://motor_api:${encodeURIComponent(senha)}@127.0.0.1:55432/motor?sslmode=disable`
}

/** Roda a semente (SEED_DEMO=on, fora de produção): troca a senha do admin demo e cria uma chave nova. */
function semear(): Demo {
  const saida = execFileSync('go', ['run', './cmd/semente'], {
    cwd: RAIZ,
    env: { ...process.env, DATABASE_URL: urlBanco(), SEED_DEMO: 'on', CGO_ENABLED: '0' },
    encoding: 'utf8',
  })
  const campo = (nome: string) => {
    const valor = saida.match(new RegExp(`^${nome}:\\s+(\\S+)$`, 'm'))?.[1]
    if (!valor) throw new Error(`semente não imprimiu "${nome}"`)
    return valor
  }
  return {
    email: campo('admin'),
    senha: campo('senha'),
    segredoTotp: campo('segredo TOTP'),
    chaveApi: campo('chave de API'),
    segredoApi: campo('segredo API'),
  }
}

function base32(texto: string): Buffer {
  const alfabeto = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const c of texto.replace(/=+$/, '').toUpperCase()) {
    const v = alfabeto.indexOf(c)
    if (v < 0) throw new Error('segredo TOTP fora de base32')
    bits += v.toString(2).padStart(5, '0')
  }
  const bytes = bits.match(/.{8}/g) ?? []
  return Buffer.from(bytes.map((b) => parseInt(b, 2)))
}

/** TOTP (RFC 6238): SHA-1, 6 dígitos, passo de 30 s — o padrão do app autenticador. */
function codigoTotp(segredo: string, agora = Date.now()): string {
  const contador = Buffer.alloc(8)
  contador.writeBigUInt64BE(BigInt(Math.floor(agora / 30_000)))
  const hmac = createHmac('sha1', base32(segredo)).update(contador).digest()
  const deslocamento = (hmac.at(-1) ?? 0) & 0x0f
  return String((hmac.readUInt32BE(deslocamento) & 0x7fffffff) % 1_000_000).padStart(6, '0')
}

/**
 * Cabeçalhos da API pública: Bearer ck_live_<id> + X-Timestamp + Idempotency-Key +
 * X-Assinatura = hex(HMAC-SHA256(segredo, ts + "." + idempotency_key + "." + corpo)).
 */
function assinar(chave: string, segredo: string, corpo: string): Record<string, string> {
  const ts = String(Math.floor(Date.now() / 1000))
  const idem = `e2e-${ts}-${Math.random().toString(36).slice(2)}`
  return {
    Authorization: `Bearer ${chave}`,
    'Content-Type': 'application/json',
    'X-Timestamp': ts,
    'Idempotency-Key': idem,
    'X-Assinatura': createHmac('sha256', segredo).update(`${ts}.${idem}.${corpo}`).digest('hex'),
  }
}

// Contra a pilha real (docker compose: nginx servindo web + API). Dono do próprio estado: a semente
// troca a senha e cria uma chave a cada execução, e a taxa nova é sorteada, então sempre há o que publicar.
const CPF_SCORE_ALTO = '000.000.004-34' // fixture do bureau simulado: score 920, cai na faixa A

test('admin publica uma faixa nova e a consulta da API pública sai com essa versão', async ({
  page,
  request,
}) => {
  // CSP do nginx: nenhuma violação em todas as telas que o fluxo abre.
  const violacoes: string[] = []
  page.on('console', (msg) => {
    if (/Content Security Policy|Refused to/i.test(msg.text())) violacoes.push(msg.text())
  })
  // Texto (não função): o tsconfig do e2e não tem os tipos do DOM.
  await page.addInitScript({
    content: `document.addEventListener('securitypolicyviolation', (e) =>
      console.error('Refused to load (CSP): ' + e.violatedDirective + ' ' + e.blockedURI))`,
  })
  const demo = semear()
  const taxaBps = 100 + Math.floor(Math.random() * 800)
  const taxaTexto = (taxaBps / 100).toFixed(2).replace('.', ',')

  await test.step('entrar com senha e TOTP', async () => {
    await page.goto('/politica')
    await page.getByLabel('E-mail').fill(demo.email)
    await page.getByLabel('Senha').fill(demo.senha)
    await page.getByRole('button', { name: 'Continuar' }).click()
    await page.getByLabel('Código de 6 números').fill(codigoTotp(demo.segredoTotp))
    await page.getByRole('button', { name: 'Entrar' }).click()
    await expect(page.getByRole('heading', { name: 'Política', level: 1 })).toBeVisible()
  })

  let versao = 0
  await test.step('editar a taxa da faixa A e publicar', async () => {
    await page.getByRole('tab', { name: /Faixas/ }).click()
    const taxa = page.getByRole('group', { name: 'Faixa A' }).getByLabel('Taxa ao mês (%)')
    await taxa.fill(taxaTexto)
    await page.getByRole('button', { name: 'Publicar', exact: true }).click()
    const dialogo = page.getByRole('alertdialog')
    await dialogo.getByLabel('Comentário (opcional)').fill(`e2e: faixa A em ${taxaTexto}%`)
    await dialogo.getByRole('button', { name: 'Publicar agora' }).click()
    const aviso = page.getByText(/Versão \d+ publicada/)
    await expect(aviso).toBeVisible()
    versao = Number((await aviso.textContent())?.match(/Versão (\d+)/)?.[1])
    expect(versao).toBeGreaterThan(0)
  })

  let consultaId = ''
  await test.step('consulta assinada pela API pública', async () => {
    const corpo = JSON.stringify({
      documento: CPF_SCORE_ALTO,
      finalidade: 'concessao_credito',
      pedido: { valor: 100_000, prazo: 12 },
    })
    const resposta = await request.post('/v1/consultas', {
      headers: assinar(demo.chaveApi, demo.segredoApi, corpo),
      data: corpo,
    })
    expect(resposta.status()).toBe(200)
    const decisao = await resposta.json()
    expect(decisao.status).toBe('aprovado')
    expect(decisao.politica.versao).toBe(versao)
    expect(decisao.oferta.taxa_mensal_bps).toBe(taxaBps)
    consultaId = decisao.id
  })

  await test.step('ver a consulta no painel com a versão nova', async () => {
    await page.getByRole('link', { name: 'Consultas' }).click()
    await page.getByLabel('CPF ou CNPJ').fill(CPF_SCORE_ALTO)
    await page.getByRole('button', { name: 'Buscar' }).click()
    await page.locator(`a[href="/consultas/${consultaId}"]`).click()
    await expect(page.getByText(`Versão ${versao}`, { exact: true })).toBeVisible()
    // A taxa aparece na oferta e no bloco Limite (mesma faixa).
    await expect(page.getByText(`${taxaTexto}% a.m.`)).toHaveCount(2)
  })

  await test.step('chave criada na tela assina a API (id no Bearer, segredo só na assinatura)', async () => {
    await page.getByRole('link', { name: 'Chaves de API' }).click()
    await page.getByLabel('Nome da nova chave').fill(`e2e ${Date.now()}`)
    await page.getByRole('button', { name: 'Criar chave' }).click()
    const dialogo = page.getByRole('alertdialog', { name: 'Sua nova chave' })
    await expect(dialogo.getByTestId('segredo')).toHaveCount(2)
    const [chave, segredo] = await dialogo.getByTestId('segredo').allTextContents()
    expect(chave).toMatch(/^ck_live_[0-9A-Z]{26}$/)
    expect(segredo).toMatch(/^[0-9a-f]{64}$/)
    await dialogo.getByRole('button', { name: 'Já guardei, fechar' }).click()

    const corpo = JSON.stringify({
      documento: CPF_SCORE_ALTO,
      finalidade: 'concessao_credito',
      pedido: { valor: 100_000, prazo: 12 },
    })
    const ok = await request.post('/v1/consultas', {
      headers: assinar(chave as string, segredo as string, corpo),
      data: corpo,
    })
    expect(ok.status()).toBe(200)
    const comSegredoNoBearer = assinar(chave as string, segredo as string, corpo)
    comSegredoNoBearer.Authorization = `Bearer ${chave}_${segredo}`
    const recusada = await request.post('/v1/consultas', { headers: comSegredoNoBearer, data: corpo })
    expect(recusada.status()).toBe(401)
  })

  await test.step('simulador: exemplo Aprovado, status 200 e "Ver no painel" abre a consulta', async () => {
    await page.getByRole('link', { name: 'Simulador de integração' }).click()
    await page.getByRole('group', { name: 'Exemplos' }).getByRole('button', { name: 'Aprovado' }).click()
    await page.getByRole('button', { name: 'Enviar consulta' }).click()
    const resposta = page.getByRole('region', { name: 'Resposta' })
    await expect(resposta.getByText('200', { exact: true })).toBeVisible()
    await expect(resposta.getByText('Aprovado', { exact: true })).toBeVisible()
    const assinatura = page
      .getByRole('region', { name: 'Requisição' })
      .getByRole('row', { name: /X-Assinatura/ })
    await expect(assinatura).toContainText(/[0-9a-f]{64}/)
    await resposta.getByRole('link', { name: /Ver no painel/ }).click()
    await expect(page).toHaveURL(/\/consultas\/cons_/)
    await expect(page.getByText(`Versão ${versao}`, { exact: true })).toBeVisible()
  })

  await test.step('simulador: consulta sem valor (pré-aprovação) sai Pré-aprovado com o limite', async () => {
    await page.getByRole('link', { name: 'Simulador de integração' }).click()
    await page.getByRole('group', { name: 'Exemplos' }).getByRole('button', { name: 'Pré-aprovação' }).click()
    await expect(page.getByLabel('Valor pedido (R$)')).toHaveValue('')
    await page.getByRole('button', { name: 'Enviar consulta' }).click()
    const resposta = page.getByRole('region', { name: 'Resposta' })
    await expect(resposta.getByText('200', { exact: true })).toBeVisible()
    await expect(resposta.getByText('Pré-aprovado', { exact: true })).toBeVisible()
    const limite = resposta.getByRole('region', { name: 'Limite' })
    await expect(limite.getByText('Valor máximo')).toBeVisible()
    await expect(limite.getByRole('row')).toHaveCount(4) // cabeçalho + 12, 24 e 36 meses
    await resposta.getByRole('link', { name: /Ver no painel/ }).click()
    await expect(page).toHaveURL(/\/consultas\/cons_/)
    await expect(page.getByText('Pré-aprovado', { exact: true }).first()).toBeVisible()
    await expect(page.getByRole('region', { name: 'Limite' })).toBeVisible()
  })

  await test.step('nenhuma violação de CSP no caminho', async () => {
    const resposta = await request.get('/')
    expect(resposta.headers()['content-security-policy']).toContain("script-src 'self'")
    expect(violacoes).toEqual([])
  })
})
