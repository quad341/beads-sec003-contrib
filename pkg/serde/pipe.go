package serde

import (
	"context"
	"fmt"
)

// Pipe copies all rows from src to dst in-process without materializing the
// full slice. It calls ExportRows, feeds the sequence to ImportRows, then
// checks the iterator error. If ImportRows returns an error, it is returned
// directly; iterator errors are only surfaced when ImportRows succeeds.
func Pipe(ctx context.Context, src RowExporter, dst RowImporter) error {
	rows, iterErr, err := src.ExportRows(ctx)
	if err != nil {
		return fmt.Errorf("serde: pipe export: %w", err)
	}
	if err := dst.ImportRows(ctx, rows); err != nil {
		return fmt.Errorf("serde: pipe import: %w", err)
	}
	if err := iterErr(); err != nil {
		return fmt.Errorf("serde: pipe export iter: %w", err)
	}
	return nil
}
