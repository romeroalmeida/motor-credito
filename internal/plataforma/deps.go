package plataforma

// Imports em branco só para o `go mod tidy` manter as dependências fixadas pela fundação.
// E (integração) apaga este arquivo quando todas estiverem em uso de verdade.
//
// Atenção: o cel-go mudou de caminho na v0.32 — importe "cel.dev/cel-go/cel", não "github.com/google/cel-go".
import (
	_ "cel.dev/cel-go/cel"
	_ "github.com/go-chi/chi/v5"
	_ "github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/oklog/ulid/v2"
	_ "github.com/pquerna/otp/totp"
	_ "github.com/testcontainers/testcontainers-go"
	_ "github.com/testcontainers/testcontainers-go/modules/postgres"
	_ "golang.org/x/crypto/argon2"
)
