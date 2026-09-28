package main

import "testing"

func TestSelectSample_ExhaustiveWhenZero(t *testing.T) {
	steps := []int{1, 2, 3, 4, 5}
	got := selectSample(steps, 0)
	if len(got) != len(steps) {
		t.Fatalf("selectSample(steps, 0) = %v, want all %d steps", got, len(steps))
	}
}

func TestSelectSample_ExhaustiveWhenNGreaterThanLen(t *testing.T) {
	steps := []int{1, 2, 3}
	got := selectSample(steps, 10)
	if len(got) != 3 {
		t.Fatalf("selectSample(steps, 10) = %v, want all 3 steps", got)
	}
}

func TestSelectSample_ReturnsExactlyN(t *testing.T) {
	steps := make([]int, 100)
	for i := range steps {
		steps[i] = i
	}
	got := selectSample(steps, 10)
	if len(got) != 10 {
		t.Fatalf("selectSample returned %d elements, want 10", len(got))
	}
}

func TestSelectSample_Deterministic(t *testing.T) {
	steps := make([]int, 37)
	for i := range steps {
		steps[i] = i
	}
	a := selectSample(steps, 7)
	b := selectSample(steps, 7)
	if len(a) != len(b) {
		t.Fatalf("length differs across calls: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("selectSample not deterministic at index %d: %v vs %v", i, a, b)
		}
	}
}

func TestSelectSample_PreservesOrder(t *testing.T) {
	steps := []int{10, 20, 30, 40, 50}
	got := selectSample(steps, 3)
	for i := 1; i < len(got); i++ {
		if got[i] <= got[i-1] {
			t.Fatalf("selectSample did not preserve order: %v", got)
		}
	}
}
