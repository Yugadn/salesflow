// Package migrations embeds the service's SQL migrations into the binary.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
