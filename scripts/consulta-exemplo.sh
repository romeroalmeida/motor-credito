#!/usr/bin/env bash
# Assina e chama POST /v1/consultas com curl. Nenhum segredo fica no repositório: a chave vem de MOTOR_CHAVE.
#
#   export MOTOR_CHAVE='ck_live_...'          # a semente (cmd/semente) imprime uma
#   scripts/consulta-exemplo.sh               # CPF de score alto do bureau simulado
#   scripts/consulta-exemplo.sh 000.000.003-53 500000 12
#
# Variáveis: MOTOR_URL (padrão http://127.0.0.1:58080), IDEMPOTENCY_KEY (padrão: uma nova a cada chamada).
set -euo pipefail

: "${MOTOR_CHAVE:?defina MOTOR_CHAVE com a chave de API completa (ck_live_<id>_<segredo>)}"
url="${MOTOR_URL:-http://127.0.0.1:58080}"
documento="${1:-000.000.004-34}"
valor="${2:-1000000}" # centavos
prazo="${3:-24}"      # meses
idem="${IDEMPOTENCY_KEY:-exemplo-$(date +%s)-$RANDOM}"

corpo=$(printf '{"documento":"%s","finalidade":"concessao_credito","pedido":{"valor":%d,"prazo":%d},"dados":{"nome":"Beatriz Ficticia Alta","email":"beatriz@exemplo.com.br","ip":"200.150.10.1"}}' \
  "$documento" "$valor" "$prazo")
timestamp=$(date +%s)
# X-Assinatura = hex(HMAC-SHA256(chave completa, timestamp + "." + corpo))
assinatura=$(printf '%s.%s' "$timestamp" "$corpo" | openssl dgst -sha256 -hmac "$MOTOR_CHAVE" -r | cut -d' ' -f1)

curl -sS -X POST "$url/v1/consultas" \
  -H "Authorization: Bearer $MOTOR_CHAVE" \
  -H "X-Timestamp: $timestamp" \
  -H "X-Assinatura: $assinatura" \
  -H "Idempotency-Key: $idem" \
  -H "Content-Type: application/json" \
  --data-binary "$corpo"
echo
