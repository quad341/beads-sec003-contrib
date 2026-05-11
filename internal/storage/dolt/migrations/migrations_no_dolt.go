//go:build no_dolt

// Package migrations — no_dolt stub variant.
//
// Under -tags no_dolt the bd binary excludes the Dolt schema migrations
// entirely. The exported helpers compile to no-ops so cmd/bd/migrate.go
// (which lists migrations for display) and the dolt/embeddeddolt store
// constructors (which run migrations on open) keep compiling unchanged.
package migrations

import "database/sql"

// RunCompatMigrations is a no-op under no_dolt.
func RunCompatMigrations(_ *sql.DB) error { return nil }

// ListCompatMigrations returns an empty slice under no_dolt.
func ListCompatMigrations() []string { return nil }
