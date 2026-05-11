//go:build no_dolt

package main

import "context"

// maybeAutoBackup is a no-op under no_dolt — Dolt-native backup is unavailable
// in this build. PG-backed projects rely on the upstream Postgres backup story
// (pg_dump, managed-DB snapshots) rather than bd's Dolt sync helper.
func maybeAutoBackup(_ context.Context) {}
