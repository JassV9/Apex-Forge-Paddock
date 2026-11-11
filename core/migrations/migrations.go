// Package migrations embeds the SQL migrations so the core binary can apply them itself.
// Files are goose migrations: reviewed, reversible, and never edited after merge.
package migrations

import "embed"

// FS holds every *.sql migration in this directory.
//
//go:embed *.sql
var FS embed.FS
