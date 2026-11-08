#!/usr/bin/env bash
# Assina e chama POST /v1/consultas com curl. Nenhum segredo fica no repositório: vêm de MOTOR_CHAVE e MOTOR_SEGREDO.
#
#   export MOTOR_CHAVE='ck_live_...'          # id público da chave (vai no Bearer)
#   export MOTOR_SEGREDO='...'                # segredo (64 hex): só assina, nunca vai em cabeçalho
#   scripts/consulta-exemplo.sh               # CPF de score alto do bureau simulado
#   scripts/consulta-exemplo.sh 000.000.003-53 500000 12
#
# A semente (cmd/semente) e a tela de chaves do painel mostram os dois.
# Variáveis: MOTOR_URL (padrão http://127.0.0.1:58080), IDEMPOTENCY_KEY (padrão: uma nova a cada chamada).
set -euo pipefail

: "${MOTOR_CHAVE:?defina MOTOR_CHAVE com o id da chave de API (ck_live_<id>)}"
: "${MOTOR_SEGREDO:?defina MOTOR_SEGREDO com o segredo da chave de API}"
url="${MOTOR_URL:-http://127.0.0.1:58080}"
documento="${1:-000.000.004-34}"
valor="${2:-1000000}" # centavos
prazo="${3:-24}"      # meses
idem="${IDEMPOTENCY_KEY:-exemplo-$(date +%s)-$RANDOM}"

corpo=$(printf '{"documento":"%s","finalidade":"concessao_credito","pedido":{"valor":%d,"prazo":%d},"dados":{"nome":"Beatriz Ficticia Alta","email":"beatriz@exemplo.com.br","ip":"200.150.10.1"}}' \
  "$documento" "$valor" "$prazo")
timestamp=$(date +%s)
# X-Assinatura = hex(HMAC-SHA256(segredo, timestamp + "." + idempotency_key + "." + corpo))
assinatura=$(printf '%s.%s.%s' "$timestamp" "$idem" "$corpo" | openssl dgst -sha256 -hmac "$MOTOR_SEGREDO" -r | cut -d' ' -f1)

curl -sS -X POST "$url/v1/consultas" \
  -H "Authorization: Bearer $MOTOR_CHAVE" \
  -H "X-Timestamp: $timestamp" \
  -H "X-Assinatura: $assinatura" \
  -H "Idempotency-Key: $idem" \
  -H "Content-Type: application/json" \
  --data-binary "$corpo"
echo
