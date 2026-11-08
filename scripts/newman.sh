#!/usr/bin/env bash
# Valida a coleção do Postman contra a pilha do compose: roda a semente (troca a senha do admin demo e cria uma
# chave nova), gera o TOTP atual e chama o newman. Imprime a saída da semente (com segredos) só no seu terminal.
#
#   docker compose up -d --build --wait api web
#   scripts/newman.sh
set -euo pipefail
cd "$(dirname "$0")/.."
set -a; . ./.env; set +a
export CGO_ENABLED=0

saida=$(DATABASE_URL="postgres://motor_api:${POSTGRES_APP_PASSWORD}@127.0.0.1:55432/motor?sslmode=disable" \
  SEED_DEMO=on AMBIENTE=desenvolvimento go run ./cmd/semente)
echo "$saida"
campo() { sed -nE "s/^$1:[[:space:]]+//p" <<<"$saida"; }

npx -y newman run postman/motor-credito.postman_collection.json -e postman/local.postman_environment.json \
  --env-var chave_id="$(campo 'chave de API')" --env-var chave_segredo="$(campo 'segredo API')" \
  --env-var email="$(campo admin)" --env-var senha="$(campo senha)" \
  --env-var totp="$(go run scripts/totp.go "$(campo 'segredo TOTP')")"
