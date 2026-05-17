package serde

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"
	"testing"
)

// helpers ---------------------------------------------------------------

func seqFromRows(rows []Row) iter.Seq[Row] {
	return func(yield func(Row) bool) {
		for _, r := range rows {
			if !yield(r) {
				return
			}
		}
	}
}

func noErr() error { return nil }

func marshalJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// WriteJSONL -----------------------------------------------------------

func TestWriteJSONL_InlinePayload(t *testing.T) {
	payload := marshalJSON(map[string]string{"title": "hello"})
	rows := seqFromRows([]Row{{Kind: KindIssue, Version: 1, Payload: payload}})

	var buf bytes.Buffer
	if err := WriteJSONL(&buf, rows, noErr); err != nil {
		t.Fatalf("WriteJSONL: %v", err)
	}

	line := strings.TrimSpace(buf.String())
	// Payload must be embedded inline, not base64-encoded.
	if !strings.Contains(line, `"title":"hello"`) {
		t.Errorf("payload not embedded inline; got: %s", line)
	}
	var out jsonlRow
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if out.Kind != KindIssue {
		t.Errorf("kind: want %q, got %q", KindIssue, out.Kind)
	}
	if out.Version != 1 {
		t.Errorf("version: want 1, got %d", out.Version)
	}
}

func TestWriteJSONL_DrainOnWriteError(t *testing.T) {
	// Track which row indices were yielded to confirm full drain even after failure.
	var yielded int
	rows := func(yield func(Row) bool) {
		for i := 0; i < 3; i++ {
			yielded++
			if !yield(Row{Kind: KindIssue, Version: 1, Payload: []byte("{}")}) {
				return
			}
		}
	}

	w := &failAfterWriter{limit: 1}
	err := WriteJSONL(w, rows, noErr)
	if err == nil {
		t.Fatal("want write error, got nil")
	}
	// All three rows must have been yielded despite the writer failing.
	if yielded != 3 {
		t.Errorf("iterator drained %d rows, want 3", yielded)
	}
}

func TestWriteJSONL_IterErrForwarded(t *testing.T) {
	sourceErr := errors.New("source blew up")
	rows := seqFromRows([]Row{{Kind: KindDep, Version: 1, Payload: []byte("{}")}})

	var buf bytes.Buffer
	err := WriteJSONL(&buf, rows, func() error { return sourceErr })
	if !errors.Is(err, sourceErr) {
		t.Errorf("want %v, got %v", sourceErr, err)
	}
}

func TestWriteJSONL_WriteErrorTakesPrecedenceOverIterErr(t *testing.T) {
	iterErrSentinel := errors.New("iter error")
	rows := seqFromRows([]Row{{Kind: KindIssue, Version: 1, Payload: []byte("{}")}})

	w := &failAfterWriter{limit: 0}
	err := WriteJSONL(w, rows, func() error { return iterErrSentinel })
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if errors.Is(err, iterErrSentinel) {
		t.Errorf("write error should take precedence over iter error, but got iter error")
	}
}

func TestWriteJSONL_EmptyIterator(t *testing.T) {
	rows := seqFromRows(nil)
	var buf bytes.Buffer
	if err := WriteJSONL(&buf, rows, noErr); err != nil {
		t.Fatalf("empty iterator: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("want empty output for empty iterator, got %q", buf.String())
	}
}

func TestWriteJSONL_MultipleRows(t *testing.T) {
	rows := seqFromRows([]Row{
		{Kind: KindIssue, Version: 1, Payload: marshalJSON(map[string]int{"n": 1})},
		{Kind: KindDep, Version: 2, Payload: marshalJSON(map[string]int{"n": 2})},
	})
	var buf bytes.Buffer
	if err := WriteJSONL(&buf, rows, noErr); err != nil {
		t.Fatalf("WriteJSONL: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), buf.String())
	}
	var first, second jsonlRow
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("decode first: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if first.Kind != KindIssue || second.Kind != KindDep {
		t.Errorf("kinds: got %q %q", first.Kind, second.Kind)
	}
}

// ReadJSONL -----------------------------------------------------------

func TestReadJSONL_EmptyLineSkip(t *testing.T) {
	input := "\n{\"kind\":\"issue\",\"version\":1,\"payload\":{}}\n\n{\"kind\":\"dep\",\"version\":2,\"payload\":{}}\n"
	seq, iterErr, err := ReadJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ReadJSONL: %v", err)
	}
	var rows []Row
	for row := range seq {
		rows = append(rows, row)
	}
	if err := iterErr(); err != nil {
		t.Fatalf("iterErr: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("want 2 rows (empty lines skipped), got %d", len(rows))
	}
}

func TestReadJSONL_MidStreamDecodeError(t *testing.T) {
	// Second line is invalid JSON.
	input := "{\"kind\":\"issue\",\"version\":1,\"payload\":{}}\nnot-json\n{\"kind\":\"dep\",\"version\":1,\"payload\":{}}\n"
	seq, iterErr, err := ReadJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ReadJSONL setup: %v", err)
	}
	var count int
	for range seq {
		count++
	}
	// Only the first row is yielded; iteration stops on decode error.
	if count != 1 {
		t.Errorf("want 1 row before decode error, got %d", count)
	}
	if err := iterErr(); err == nil {
		t.Fatal("want decode error, got nil")
	}
}

func TestReadJSONL_PayloadPreserved(t *testing.T) {
	payload := `{"title":"round-trip","x":42}`
	input := fmt.Sprintf("{\"kind\":\"issue\",\"version\":3,\"payload\":%s}\n", payload)
	seq, iterErr, err := ReadJSONL(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ReadJSONL: %v", err)
	}
	var rows []Row
	for row := range seq {
		rows = append(rows, row)
	}
	if err := iterErr(); err != nil {
		t.Fatalf("iterErr: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.Kind != KindIssue {
		t.Errorf("kind: want %q, got %q", KindIssue, r.Kind)
	}
	if r.Version != 3 {
		t.Errorf("version: want 3, got %d", r.Version)
	}
	var got, want map[string]any
	if err := json.Unmarshal(r.Payload, &got); err != nil {
		t.Fatalf("payload unmarshal: %v", err)
	}
	if err := json.Unmarshal([]byte(payload), &want); err != nil {
		t.Fatalf("want unmarshal: %v", err)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("payload mismatch: got %v want %v", got, want)
	}
}

func TestReadJSONL_ScannerError(t *testing.T) {
	seq, iterErr, err := ReadJSONL(&errorReader{errAfter: 10})
	if err != nil {
		t.Fatalf("ReadJSONL setup: %v", err)
	}
	for range seq {
		// consume
	}
	if err := iterErr(); err == nil {
		t.Fatal("want scanner error, got nil")
	}
}

// Pipe -----------------------------------------------------------------

func TestPipe_HappyPath(t *testing.T) {
	src := &mockExporter{rows: []Row{
		{Kind: KindIssue, Version: 1, Payload: []byte(`{}`)},
		{Kind: KindLabel, Version: 1, Payload: []byte(`{}`)},
	}}
	dst := &mockImporter{}
	if err := Pipe(context.Background(), src, dst); err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	if len(dst.received) != 2 {
		t.Errorf("want 2 rows imported, got %d", len(dst.received))
	}
}

func TestPipe_ImportError(t *testing.T) {
	importErr := errors.New("import failed")
	src := &mockExporter{rows: []Row{{Kind: KindIssue, Version: 1, Payload: []byte(`{}`)}}}
	dst := &mockImporter{err: importErr}
	err := Pipe(context.Background(), src, dst)
	if !errors.Is(err, importErr) {
		t.Errorf("want %v, got %v", importErr, err)
	}
}

func TestPipe_IterErrAfterImportSuccess(t *testing.T) {
	iterSentinel := errors.New("source error after export")
	src := &mockExporter{
		rows:    []Row{{Kind: KindIssue, Version: 1, Payload: []byte(`{}`)}},
		iterErr: iterSentinel,
	}
	dst := &mockImporter{}
	err := Pipe(context.Background(), src, dst)
	if !errors.Is(err, iterSentinel) {
		t.Errorf("want %v, got %v", iterSentinel, err)
	}
}

func TestPipe_ImportErrorTakesPrecedenceOverIterErr(t *testing.T) {
	importErr := errors.New("import failed")
	iterSentinel := errors.New("iter err")
	src := &mockExporter{
		rows:    []Row{{Kind: KindIssue, Version: 1, Payload: []byte(`{}`)}},
		iterErr: iterSentinel,
	}
	dst := &mockImporter{err: importErr}
	err := Pipe(context.Background(), src, dst)
	if !errors.Is(err, importErr) {
		t.Errorf("import error should take precedence; want %v, got %v", importErr, err)
	}
}

func TestPipe_ExportInitError(t *testing.T) {
	exportErr := errors.New("export init failed")
	src := &mockExporter{exportErr: exportErr}
	dst := &mockImporter{}
	err := Pipe(context.Background(), src, dst)
	if !errors.Is(err, exportErr) {
		t.Errorf("want %v, got %v", exportErr, err)
	}
}

// test helpers ----------------------------------------------------------

type failAfterWriter struct {
	limit int
	n     int
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.n >= w.limit {
		return 0, errors.New("write: intentional failure")
	}
	w.n++
	return len(p), nil
}

type errorReader struct {
	errAfter int
	n        int
}

func (r *errorReader) Read(p []byte) (int, error) {
	if r.n >= r.errAfter {
		return 0, fmt.Errorf("read: intentional error at byte %d", r.n)
	}
	n := min(len(p), r.errAfter-r.n)
	for i := range n {
		p[i] = 'x'
	}
	r.n += n
	return n, nil
}

var _ io.Reader = (*errorReader)(nil)

type mockExporter struct {
	rows      []Row
	iterErr   error
	exportErr error
}

func (m *mockExporter) ExportRows(_ context.Context) (iter.Seq[Row], func() error, error) {
	if m.exportErr != nil {
		return nil, nil, m.exportErr
	}
	return seqFromRows(m.rows), func() error { return m.iterErr }, nil
}

type mockImporter struct {
	received []Row
	err      error
}

func (m *mockImporter) ImportRows(_ context.Context, rows iter.Seq[Row]) error {
	if m.err != nil {
		for range rows {
		}
		return m.err
	}
	for row := range rows {
		m.received = append(m.received, row)
	}
	return nil
}
