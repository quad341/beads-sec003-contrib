package serde

import (
	"encoding/json"
	"fmt"
	"io"
	"iter"
)

// WriteJSONL writes each Row as a JSON line to w.
// After the sequence is exhausted, iterErr is called; any error it returns
// is wrapped and returned so callers can distinguish decode errors from
// iterator errors.
func WriteJSONL(w io.Writer, rows iter.Seq[Row], iterErr func() error) error {
	enc := json.NewEncoder(w)
	for row := range rows {
		if err := enc.Encode(row); err != nil {
			return fmt.Errorf("serde: write jsonl: %w", err)
		}
	}
	if iterErr != nil {
		if err := iterErr(); err != nil {
			return fmt.Errorf("serde: write jsonl iter: %w", err)
		}
	}
	return nil
}

// ReadJSONL returns a push iterator over JSONL-encoded Rows read from r.
// The returned iterErr function reports any decode error that terminated
// the sequence; it returns nil if the stream ended cleanly.
// The initial error return is non-nil only for setup failures (none currently).
func ReadJSONL(r io.Reader) (iter.Seq[Row], func() error, error) {
	dec := json.NewDecoder(r)
	var decErr error
	seq := func(yield func(Row) bool) {
		for dec.More() {
			var row Row
			if err := dec.Decode(&row); err != nil {
				decErr = fmt.Errorf("serde: read jsonl: %w", err)
				return
			}
			if !yield(row) {
				return
			}
		}
	}
	return seq, func() error { return decErr }, nil
}
