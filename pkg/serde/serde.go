// Package serde defines the portable row format and exporter/importer
// interfaces used to migrate issue data between backends.
package serde

import (
	"context"
	"iter"
)

// RowKind identifies the domain entity type carried in a Row's Payload.
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

// Row is a single portable record in the serde stream.
// Payload contains the JSON-encoded entity body; its schema is versioned by Version.
type Row struct {
	Kind    RowKind
	Version int
	Payload []byte // JSON-encoded entity body
}

// RowExporter produces a stream of Rows from a backend.
// Callers range over the returned iter.Seq[Row]; after the iterator is
// exhausted they must call iterErr() to check for mid-stream errors.
// The top-level err covers initialization failures (no rows will be produced).
type RowExporter interface {
	ExportRows(ctx context.Context) (rows iter.Seq[Row], iterErr func() error, err error)
}

// RowImporter consumes a stream of Rows into a backend.
// Implementations may batch writes in transactions of ~500 rows with
// upsert semantics to balance throughput and atomicity.
type RowImporter interface {
	ImportRows(ctx context.Context, rows iter.Seq[Row]) error
}
