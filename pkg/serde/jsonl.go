package serde

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"iter"
)

// jsonlRow is the on-wire JSON shape for a single JSONL record.
// Payload is kept as json.RawMessage so the entity body is embedded
// inline rather than base64-encoded.
type jsonlRow struct {
	Kind    RowKind         `json:"kind"`
	Version int             `json:"version"`
	Payload json.RawMessage `json:"payload"`
}

// WriteJSONL writes rows as newline-delimited JSON to w, one Row per line.
// After the iterator is exhausted it calls iterErr() to check for a
// source-side error; a non-nil result is returned if no write error occurred.
// Write errors take precedence over iterator errors.
func WriteJSONL(w io.Writer, rows iter.Seq[Row], iterErr func() error) error {
	enc := json.NewEncoder(w)
	var writeErr error
	for row := range rows {
		if writeErr != nil {
			// drain the iterator so the source can clean up, but stop encoding
			continue
		}
		jr := jsonlRow{
			Kind:    row.Kind,
			Version: row.Version,
			Payload: json.RawMessage(row.Payload),
		}
		writeErr = enc.Encode(jr)
	}
	if writeErr != nil {
		return writeErr
	}
	return iterErr()
}

// ReadJSONL reads newline-delimited JSON from r and returns an iterator
// over the rows it contains. The caller must invoke iterErr() after the
// iterator is exhausted to check for mid-stream decode or scan errors.
// The returned top-level error is reserved for setup failures.
func ReadJSONL(r io.Reader) (iter.Seq[Row], func() error, error) {
	var scanErr error
	seq := func(yield func(Row) bool) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var jr jsonlRow
			if err := json.Unmarshal(line, &jr); err != nil {
				scanErr = fmt.Errorf("serde: decode JSONL row: %w", err)
				return
			}
			if !yield(Row{
				Kind:    jr.Kind,
				Version: jr.Version,
				Payload: []byte(jr.Payload),
			}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			scanErr = fmt.Errorf("serde: scan JSONL: %w", err)
		}
	}
	return seq, func() error { return scanErr }, nil
}
