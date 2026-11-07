# CGO desligado: binário estático e sem depender do toolchain C da máquina.
export CGO_ENABLED=0

.PHONY: dev banco test test-curto lint web-install

banco:
	docker compose up -d postgres

dev: banco
	@echo "API em :8080 e painel em :5173 (Ctrl+C encerra os dois)"
	@trap 'kill 0' INT TERM; \
	  (set -a; [ -f .env ] && . ./.env; set +a; go run ./cmd/api) & \
	  npm --prefix web run dev & \
	  wait

test:
	go test ./...
	npm --prefix web test

# Sem os testes de integração (não precisa de Docker).
test-curto:
	go test -short ./...

lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "rode gofmt -w" && exit 1)
	npm --prefix web run lint

web-install:
	npm --prefix web ci
