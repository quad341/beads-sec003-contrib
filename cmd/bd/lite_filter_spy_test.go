//go:build cgo && dolt_only

package main

import (
	"context"
	"sync"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/types"
)

// filterCapturingStore is the shared spy used by the be-uwvs.5 round-trip
// gate tests:
//
//   - TestExportFilterIsAlwaysFull         (cmd/bd/export_test.go)
//   - TestExportAutoFilterIsAlwaysFull     (cmd/bd/export_auto_test.go)
//   - TestMigrateIssuesFilterIsAlwaysFull  (cmd/bd/migrate_issues_test.go)
//   - TestJiraSyncFilterIsAlwaysFull       (cmd/bd/jira_test.go)
//
// It embeds *dolt.DoltStore (concrete) rather than the narrow
// storage.Storage interface so that cmd/bd's capability assertions
// (mustAnnot, mustDeps, mustConfig — see storage_caps.go) continue to
// resolve to the underlying Dolt-backed capabilities. Wrapping with the
// bare interface would satisfy storage.Storage at compile time but break
// the runtime AnnotationStore / DependencyQueryStore type assertions the
// commands under test rely on.
//
// SearchIssues records every filter received and then delegates to the
// embedded store so the caller sees real query results. Tests call
// LastFilter() / AllFilters() to assert on the gate invariant:
//
//   - filter.Lite MUST be false on every call (round-trip integrity).
//   - filter.Limit MUST be 0 on the main round-trip call (unlimited).
//
// Composability with be-x42v: that bead's parallel gate work adds
// MaxRows / MaxRowsSource assertions on the same spy. Recording every
// filter (not just the last) gives that follow-up free coverage of
// multi-call paths like migrate-issues without re-touching the spy.
//
// The spy is intentionally tagged `cgo && dolt_only` because every gate
// test that consumes it needs a Dolt-backed underlying store (per the
// existing cmd/bd test patterns at e.g. export_test.go:1).
type filterCapturingStore struct {
	*dolt.DoltStore

	mu       sync.Mutex
	captured []types.IssueFilter
}

// newFilterCapturingStore wraps an existing *dolt.DoltStore in a spy.
// Callers reach the inner store via the embedded field if they need to
// seed data outside the spy's SearchIssues capture (e.g., via DB().Exec).
func newFilterCapturingStore(inner *dolt.DoltStore) *filterCapturingStore {
	return &filterCapturingStore{DoltStore: inner}
}

// SearchIssues records the filter then delegates to the embedded Dolt
// store. The filter is captured before delegation so a panicking
// underlying call (which shouldn't happen with Dolt) still leaves the
// captured slice consistent for the test's diagnostic message.
func (s *filterCapturingStore) SearchIssues(ctx context.Context, query string, filter types.IssueFilter) ([]*types.Issue, error) {
	s.mu.Lock()
	s.captured = append(s.captured, filter)
	s.mu.Unlock()
	return s.DoltStore.SearchIssues(ctx, query, filter)
}

// LastFilter returns the most recent filter recorded by SearchIssues.
// Returns the zero-value types.IssueFilter{} (and len(s.captured)==0
// observable via Calls()) if SearchIssues was never invoked — callers
// should assert on Calls() > 0 before reading the filter.
func (s *filterCapturingStore) LastFilter() types.IssueFilter {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.captured) == 0 {
		return types.IssueFilter{}
	}
	return s.captured[len(s.captured)-1]
}

// AllFilters returns a snapshot of every filter SearchIssues received.
// The returned slice is a copy so callers can iterate without holding
// the spy lock.
func (s *filterCapturingStore) AllFilters() []types.IssueFilter {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]types.IssueFilter, len(s.captured))
	copy(out, s.captured)
	return out
}

// Calls returns the number of times SearchIssues has been invoked on
// this spy. Tests use it to gate that the spy was actually exercised
// before asserting on the captured filter — a test that never reaches
// SearchIssues would otherwise pass vacuously.
func (s *filterCapturingStore) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.captured)
}

// Compile-time assurance that filterCapturingStore satisfies
// storage.Storage. The embedded *dolt.DoltStore provides every required
// method; this assertion fails at build time if either the embed is
// changed or storage.Storage adds a method DoltStore doesn't satisfy.
var _ storage.Storage = (*filterCapturingStore)(nil)
