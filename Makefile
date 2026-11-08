# CGO desligado: binário estático e sem depender do toolchain C da máquina.
export CGO_ENABLED=0

.PHONY: dev banco test test-curto lint web-install e2e

banco:
	docker compose up -d postgres

dev: banco
	@echo "API em :8080 e painel em :5179 (Ctrl+C encerra os dois)"
	@trap 'kill 0' INT TERM; \
	  (set -a; [ -f .env ] && . ./.env; set +a; go run ./cmd/api) & \
	  API_DEV=http://127.0.0.1:8080 npm --prefix web run dev & \
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

# e2e contra a pilha real: sobe postgres + api + web (nginx na 55173) e roda o spec, que chama a semente.
e2e:
	docker compose up -d --build --wait api web
	set -a; . ./.env; set +a; AMBIENTE=desenvolvimento npm --prefix web run test:e2e
