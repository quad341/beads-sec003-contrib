package main

import (
	"context"
	"encoding/json"
	"fmt"
)

// OracleReadFunc reads the tool-of-record's answer for one (commit, issue)
// pair. ReplayFunc replays the same mutation through the integration binary
// under test and returns the resulting row. Both are plain string-keyed rows
// (a dolt CSV row, column name -> value).
type OracleReadFunc func(ctx context.Context) (map[string]string, error)
type ReplayFunc func(ctx context.Context) (map[string]string, error)

// ReplayAndCompare reads the oracle's answer, then replays the mutation, and
// only then compares the two. AC3 requires oracle-before-replay to be
// structurally enforced rather than a convention callers might get backwards:
// a failing readOracle short-circuits before replay is ever invoked, so a
// broken oracle read can never be masked by (or blamed on) a replay side
// effect.
func ReplayAndCompare(ctx context.Context, readOracle OracleReadFunc, replay ReplayFunc) (CompareResult, error) {
	oracleRow, err := readOracle(ctx)
	if err != nil {
		return CompareResult{}, fmt.Errorf("replay and compare: read oracle: %w", err)
	}

	candidateRow, err := replay(ctx)
	if err != nil {
		return CompareResult{}, fmt.Errorf("replay and compare: replay: %w", err)
	}

	oracleJSON, err := json.Marshal(filterComparable(oracleRow))
	if err != nil {
		return CompareResult{}, fmt.Errorf("replay and compare: marshal oracle row: %w", err)
	}
	candidateJSON, err := json.Marshal(filterComparable(candidateRow))
	if err != nil {
		return CompareResult{}, fmt.Errorf("replay and compare: marshal candidate row: %w", err)
	}

	result, err := Compare(oracleJSON, candidateJSON)
	if err != nil {
		return CompareResult{}, fmt.Errorf("replay and compare: compare: %w", err)
	}
	return result, nil
}

// comparableColumnsExclude are columns that legitimately differ between an
// oracle read and a fresh replay without indicating a real mismatch:
// content_hash/row_lock change as a side effect of any write (mirrors
// mutation-translator's ignoredColumns), is_blocked is recomputed rather
// than replayed directly, and the rest of timestampKeys (updated_at,
// created_at, closed_at, last_activity, due_at, defer_until, compacted_at)
// are wall-clock values stamped at write time: the oracle's mutations are
// all arranged up front and only replayed later, so an oracle write and its
// replay necessarily land at different real times, and every replay would
// otherwise mismatch on these columns alone regardless of whether the
// replayed content is correct. Confirmed empirically: an unexcluded
// created_at/closed_at was the actual cause of an "unexpected mismatch on
// faithful replay" failure on every single replay step (be-sodi8 notes).
// Reuses timestampKeys instead of re-listing it, since compare.go's
// classifyMismatch already established exactly this set as the
// "as-of-mismatch" family.
var comparableColumnsExclude = func() map[string]bool {
	out := map[string]bool{
		"content_hash": true,
		"row_lock":     true,
		"is_blocked":   true,
	}
	for k := range timestampKeys {
		out[k] = true
	}
	return out
}()

// filterComparable returns a copy of row with incidental columns removed, or
// nil for nil input.
func filterComparable(row map[string]string) map[string]string {
	if row == nil {
		return nil
	}
	out := make(map[string]string, len(row))
	for k, v := range row {
		if comparableColumnsExclude[k] {
			continue
		}
		out[k] = v
	}
	return out
}
