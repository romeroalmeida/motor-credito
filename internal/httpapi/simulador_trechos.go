package httpapi

import (
	"encoding/json"
	"strings"
)

// trechos são a mesma requisição em curl, Node e Python, prontos para copiar. O segredo nunca entra: cada trecho
// lê MOTOR_SEGREDO do ambiente e calcula a assinatura, com timestamp e Idempotency-Key novos a cada execução.
type trechos struct {
	Curl   string `json:"curl"`
	Node   string `json:"node"`
	Python string `json:"python"`
}

func gerarTrechos(url, authorization, corpo string) trechos {
	return trechos{
		Curl:   trechoCurl(url, authorization, corpo),
		Node:   trechoNode(url, authorization, corpo),
		Python: trechoPython(url, authorization, corpo),
	}
}

// aspasShell põe o valor entre aspas simples: nada se expande dentro delas, e a própria aspa é fechada,
// escapada e reaberta.
func aspasShell(v string) string { return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'" }

// literal devolve o texto como string JSON, que também é literal válido em JavaScript e em Python.
func literal(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func trechoCurl(url, authorization, corpo string) string {
	return `# Defina MOTOR_SEGREDO com o segredo mostrado na criação da chave.
CORPO=` + aspasShell(corpo) + `
TS=$(date +%s)
IDEM=$(uuidgen)
ASSINATURA=$(printf '%s' "$TS.$IDEM.$CORPO" | openssl dgst -sha256 -hmac "$MOTOR_SEGREDO" | awk '{print $NF}')

curl -sS -X POST ` + aspasShell(url) + ` \
  -H ` + aspasShell("Authorization: "+authorization) + ` \
  -H 'Content-Type: application/json' \
  -H "X-Timestamp: $TS" \
  -H "Idempotency-Key: $IDEM" \
  -H "X-Assinatura: $ASSINATURA" \
  --data-raw "$CORPO"
`
}

func trechoNode(url, authorization, corpo string) string {
	return `// Node 18+. Defina MOTOR_SEGREDO com o segredo mostrado na criação da chave.
import { createHmac, randomUUID } from 'node:crypto'

const url = ` + literal(url) + `
const corpo = ` + literal(corpo) + `
const ts = String(Math.floor(Date.now() / 1000))
const idem = randomUUID() // uma por operação: repetir a mesma devolve a mesma decisão
const assinatura = createHmac('sha256', process.env.MOTOR_SEGREDO)
  .update(` + "`${ts}.${idem}.${corpo}`" + `)
  .digest('hex')

const resposta = await fetch(url, {
  method: 'POST',
  headers: {
    Authorization: ` + literal(authorization) + `,
    'Content-Type': 'application/json',
    'X-Timestamp': ts,
    'Idempotency-Key': idem,
    'X-Assinatura': assinatura,
  },
  body: corpo,
})
console.log(resposta.status, await resposta.json())
`
}

func trechoPython(url, authorization, corpo string) string {
	return `# Python 3, só biblioteca padrão. Defina MOTOR_SEGREDO com o segredo mostrado na criação da chave.
import hashlib, hmac, json, os, time, uuid, urllib.error, urllib.request

url = ` + literal(url) + `
corpo = ` + literal(corpo) + `
ts = str(int(time.time()))
idem = str(uuid.uuid4())  # uma por operação: repetir a mesma devolve a mesma decisão
assinatura = hmac.new(os.environ["MOTOR_SEGREDO"].encode(), f"{ts}.{idem}.{corpo}".encode(), hashlib.sha256).hexdigest()

requisicao = urllib.request.Request(url, data=corpo.encode(), method="POST", headers={
    "Authorization": ` + literal(authorization) + `,
    "Content-Type": "application/json",
    "X-Timestamp": ts,
    "Idempotency-Key": idem,
    "X-Assinatura": assinatura,
})
try:
    with urllib.request.urlopen(requisicao) as resposta:
        print(resposta.status, json.load(resposta))
except urllib.error.HTTPError as erro:
    print(erro.code, erro.read().decode())
`
}
