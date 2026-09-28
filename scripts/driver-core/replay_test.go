package main

import (
	"context"
	"errors"
	"testing"
)

func TestReplayAndCompare_NeverReplaysWhenOracleReadFails(t *testing.T) {
	replayCalled := false
	oracleErr := errors.New("oracle boom")

	_, err := ReplayAndCompare(context.Background(),
		func(ctx context.Context) (map[string]string, error) {
			return nil, oracleErr
		},
		func(ctx context.Context) (map[string]string, error) {
			replayCalled = true
			return map[string]string{}, nil
		},
	)
	if err == nil {
		t.Fatalf("expected error when oracle read fails")
	}
	if replayCalled {
		t.Fatalf("replay was invoked despite oracle read failing -- ordering not structurally enforced (AC3)")
	}
}

func TestReplayAndCompare_CallsOracleBeforeReplay(t *testing.T) {
	var order []string

	_, err := ReplayAndCompare(context.Background(),
		func(ctx context.Context) (map[string]string, error) {
			order = append(order, "oracle")
			return map[string]string{"id": "X-1"}, nil
		},
		func(ctx context.Context) (map[string]string, error) {
			order = append(order, "replay")
			return map[string]string{"id": "X-1"}, nil
		},
	)
	if err != nil {
		t.Fatalf("ReplayAndCompare: %v", err)
	}
	if len(order) != 2 || order[0] != "oracle" || order[1] != "replay" {
		t.Fatalf("call order = %v, want [oracle replay]", order)
	}
}

func TestReplayAndCompare_MatchedWhenRowsEquivalent(t *testing.T) {
	row := map[string]string{"id": "X-1", "title": "hello"}
	result, err := ReplayAndCompare(context.Background(),
		func(ctx context.Context) (map[string]string, error) { return row, nil },
		func(ctx context.Context) (map[string]string, error) { return row, nil },
	)
	if err != nil {
		t.Fatalf("ReplayAndCompare: %v", err)
	}
	if !result.Matched {
		t.Fatalf("expected Matched=true for identical rows")
	}
}

func TestReplayAndCompare_MismatchWhenRowsDiffer(t *testing.T) {
	result, err := ReplayAndCompare(context.Background(),
		func(ctx context.Context) (map[string]string, error) {
			return map[string]string{"id": "X-1", "title": "hello"}, nil
		},
		func(ctx context.Context) (map[string]string, error) {
			return map[string]string{"id": "X-1", "title": "goodbye"}, nil
		},
	)
	if err != nil {
		t.Fatalf("ReplayAndCompare: %v", err)
	}
	if result.Matched {
		t.Fatalf("expected Matched=false")
	}
	if result.Mismatch == nil {
		t.Fatalf("expected non-nil Mismatch")
	}
}

func TestReplayAndCompare_ErrorsWhenReplayFails(t *testing.T) {
	replayErr := errors.New("replay boom")
	_, err := ReplayAndCompare(context.Background(),
		func(ctx context.Context) (map[string]string, error) {
			return map[string]string{"id": "X-1"}, nil
		},
		func(ctx context.Context) (map[string]string, error) {
			return nil, replayErr
		},
	)
	if err == nil {
		t.Fatalf("expected error when replay fails")
	}
}

func TestFilterComparable_ExcludesIncidentalColumns(t *testing.T) {
	row := map[string]string{
		"id": "X-1", "title": "hello",
		"content_hash": "abc", "updated_at": "2026-09-27T00:00:00Z",
		"row_lock": "1", "is_blocked": "false",
	}
	got := filterComparable(row)
	for _, excluded := range []string{"content_hash", "updated_at", "row_lock", "is_blocked"} {
		if _, ok := got[excluded]; ok {
			t.Errorf("filterComparable kept incidental column %q", excluded)
		}
	}
	if got["id"] != "X-1" || got["title"] != "hello" {
		t.Errorf("filterComparable dropped a meaningful column: %+v", got)
	}
}

func TestFilterComparable_NilInput(t *testing.T) {
	if got := filterComparable(nil); got != nil {
		t.Errorf("filterComparable(nil) = %+v, want nil", got)
	}
}
