// Package serde defines the row-level serialization contracts for cross-backend
// migration and export: Row, RowKind, RowExporter, and RowImporter.
package serde

import (
	"context"
	"iter"
)

// RowKind identifies the entity type encoded in a Row's Payload.
type RowKind string

const (
	KindIssue   RowKind = "issue"
	KindDep     RowKind = "dep"
	KindLabel   RowKind = "label"
	KindComment RowKind = "comment"
	KindEvent   RowKind = "event"
	KindMemory  RowKind = "memory"
	KindMol     RowKind = "mol"
	KindWisp    RowKind = "wisp"
)

// Row is a single serialized entity in a migration or export stream.
// Payload is the JSON-encoded form of the entity at the given schema Version.
type Row struct {
	Kind    RowKind `json:"kind"`
	Version int     `json:"version"`
	Payload []byte  `json:"payload"`
}

// RowExporter streams rows from a storage backend for export or migration.
// After ranging over the returned iter.Seq[Row], callers must invoke iterErr()
// to detect any error that terminated the sequence early.
type RowExporter interface {
	ExportRows(ctx context.Context) (rows iter.Seq[Row], iterErr func() error, err error)
}

// RowImporter ingests a stream of rows into a storage backend.
// Implementations may batch in transactions with upsert semantics.
type RowImporter interface {
	ImportRows(ctx context.Context, rows iter.Seq[Row]) error
}
