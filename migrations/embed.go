// Package migrations embeds the SQL migration files so they ship inside the
// binary (needed to provision new tenant schemas at runtime).
package migrations

import "embed"

// FS contains the public/ and tenant/ migration directories.
//
//go:embed public tenant
var FS embed.FS
