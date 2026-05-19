package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
)

// --- SliceIter[T] lifecycle tests ---

// TestSliceIter_HappyPath verifies the typical Next/Value/Err/Close cycle.
func TestSliceIter_HappyPath(t *testing.T) {
	ctx := context.Background()
	items := []*int{intPtr(1), intPtr(2), intPtr(3)}
	it := storage.NewSliceIter(items)

	var collected []int
	for it.Next(ctx) {
		v := it.Value()
		if v == nil {
			t.Fatal("Value() returned nil during iteration")
		}
		collected = append(collected, *v)
	}
	if err := it.Err(); err != nil {
		t.Errorf("Err() = %v, want nil", err)
	}
	if err := it.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}

	if len(collected) != 3 || collected[0] != 1 || collected[1] != 2 || collected[2] != 3 {
		t.Errorf("collected = %v, want [1 2 3]", collected)
	}
}

// TestSliceIter_Exhaustion verifies Value() returns nil after iteration ends.
func TestSliceIter_Exhaustion(t *testing.T) {
	ctx := context.Background()
	it := storage.NewSliceIter([]*int{intPtr(42)})

	it.Next(ctx)
	_ = it.Value() // consume the one item

	// After exhaustion, Next returns false.
	if it.Next(ctx) {
		t.Error("Next() should return false after exhaustion")
	}
	if v := it.Value(); v != nil {
		t.Errorf("Value() after exhaustion = %v, want nil", v)
	}
}

// TestSliceIter_DoubleClose verifies Close is idempotent.
func TestSliceIter_DoubleClose(t *testing.T) {
	it := storage.NewSliceIter([]*int{intPtr(1)})
	if err := it.Close(); err != nil {
		t.Fatalf("first Close() = %v, want nil", err)
	}
	if err := it.Close(); err != nil {
		t.Fatalf("second Close() = %v, want nil", err)
	}
}

// TestSliceIter_NextAfterClose verifies Next returns false after Close.
func TestSliceIter_NextAfterClose(t *testing.T) {
	ctx := context.Background()
	it := storage.NewSliceIter([]*int{intPtr(1), intPtr(2)})
	it.Close() //nolint:errcheck
	if it.Next(ctx) {
		t.Error("Next() should return false after Close()")
	}
}

// TestSliceIter_Empty verifies an empty iterator exhausts immediately.
func TestSliceIter_Empty(t *testing.T) {
	ctx := context.Background()
	it := storage.NewSliceIter([]*int{})
	if it.Next(ctx) {
		t.Error("Next() on empty SliceIter should return false immediately")
	}
	if v := it.Value(); v != nil {
		t.Errorf("Value() on empty iterator = %v, want nil", v)
	}
}

// --- ForEach tests ---

// TestForEach_Drains verifies ForEach collects all values.
func TestForEach_Drains(t *testing.T) {
	ctx := context.Background()
	items := []*string{strPtr("a"), strPtr("b"), strPtr("c")}
	it := storage.NewSliceIter(items)

	var out []string
	if err := storage.ForEach(ctx, it, func(s *string) error {
		out = append(out, *s)
		return nil
	}); err != nil {
		t.Fatalf("ForEach error: %v", err)
	}
	if len(out) != 3 || out[0] != "a" || out[1] != "b" || out[2] != "c" {
		t.Errorf("ForEach collected %v, want [a b c]", out)
	}
}

// TestForEach_PropagatesFnError verifies that ForEach stops on fn error.
func TestForEach_PropagatesFnError(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("short-circuit")
	items := []*int{intPtr(1), intPtr(2), intPtr(3)}
	it := storage.NewSliceIter(items)

	count := 0
	err := storage.ForEach(ctx, it, func(_ *int) error {
		count++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("ForEach error: got %v, want sentinel", err)
	}
	if count != 1 {
		t.Errorf("fn called %d times before error, want 1", count)
	}
}

// TestForEach_AlwaysCallsClose verifies Close is called even on fn error.
func TestForEach_AlwaysCallsClose(t *testing.T) {
	ctx := context.Background()
	mc := &mockCloseCountIter{items: []*int{intPtr(1), intPtr(2)}}

	_ = storage.ForEach(ctx, mc, func(_ *int) error {
		return errors.New("boom")
	})
	if mc.closed != 1 {
		t.Errorf("Close called %d times, want 1", mc.closed)
	}
}

// --- Collect tests ---

// TestCollect_ReturnsInOrder verifies Collect drains in iteration order.
func TestCollect_ReturnsInOrder(t *testing.T) {
	ctx := context.Background()
	items := []*int{intPtr(10), intPtr(20), intPtr(30)}
	it := storage.NewSliceIter(items)

	got, err := storage.Collect(ctx, it)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if len(got) != 3 || *got[0] != 10 || *got[1] != 20 || *got[2] != 30 {
		t.Errorf("Collect = %v, want [10 20 30]", got)
	}
}

// TestCollect_EmptyReturnsNil verifies Collect returns nil (not empty slice) for empty iter.
func TestCollect_EmptyReturnsNil(t *testing.T) {
	ctx := context.Background()
	it := storage.NewSliceIter([]*int{})
	got, err := storage.Collect(ctx, it)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if got != nil {
		t.Errorf("Collect on empty iter = %v, want nil", got)
	}
}

// TestCollect_PropagatesErr verifies Collect surfaces iterator errors.
func TestCollect_PropagatesErr(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("iter error")
	me := &mockErrIter{err: sentinel}

	_, err := storage.Collect(ctx, me)
	if !errors.Is(err, sentinel) {
		t.Errorf("Collect error = %v, want sentinel", err)
	}
}

// --- helpers ---

func intPtr(n int) *int       { return &n }
func strPtr(s string) *string { return &s }

// mockCloseCountIter counts how many times Close is called.
type mockCloseCountIter struct {
	items  []*int
	idx    int
	closed int
}

func (m *mockCloseCountIter) Next(_ context.Context) bool {
	m.idx++
	return m.idx < len(m.items)
}
func (m *mockCloseCountIter) Value() *int { return m.items[m.idx] }
func (m *mockCloseCountIter) Err() error  { return nil }
func (m *mockCloseCountIter) Close() error {
	m.closed++
	return nil
}

// mockErrIter returns an error from Err() after exhaustion.
type mockErrIter struct {
	err error
}

func (m *mockErrIter) Next(_ context.Context) bool { return false }
func (m *mockErrIter) Value() *int                 { return nil }
func (m *mockErrIter) Err() error                  { return m.err }
func (m *mockErrIter) Close() error                { return nil }
