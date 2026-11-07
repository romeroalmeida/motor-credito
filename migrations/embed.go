// Package migrations embute os arquivos SQL aplicados por internal/plataforma/migrar, em ordem de nome.
package migrations

import "embed"

// FS contém os *.sql desta pasta.
//
//go:embed *.sql
var FS embed.FS
