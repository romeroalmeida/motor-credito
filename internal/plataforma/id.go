package plataforma

import "github.com/oklog/ulid/v2"

// NovoID devolve "<prefixo>_<ULID>", ordenável por tempo. Ex.: NovoID("cons") → "cons_01JC...".
func NovoID(prefixo string) string {
	return prefixo + "_" + ulid.Make().String()
}
