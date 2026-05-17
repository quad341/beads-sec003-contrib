package serde

import "context"

// Pipe copies all rows from src to dst in a single in-process pass.
// It is a convenience shortcut for in-process backend migration; for
// cross-process or cross-machine migration prefer WriteJSONL + ReadJSONL.
//
// Error priority: if ImportRows succeeds and the source iterator reports
// an error via iterErr(), that source error is returned.  If ImportRows
// itself fails, its error is returned immediately.
func Pipe(ctx context.Context, src RowExporter, dst RowImporter) error {
	rows, iterErr, err := src.ExportRows(ctx)
	if err != nil {
		return err
	}
	if err := dst.ImportRows(ctx, rows); err != nil {
		return err
	}
	return iterErr()
}
