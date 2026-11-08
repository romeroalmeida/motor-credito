# Binário estático da API (cmd/api é criado pelo agente E).
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /api ./cmd/api \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /retencao ./cmd/retencao

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /api /api
COPY --from=build /retencao /retencao
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/api"]
