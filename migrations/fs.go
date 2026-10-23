package migrations

import "embed"

// FS menyediakan seluruh berkas migrasi SQL secara embedded.
//
//go:embed *.sql
var FS embed.FS
