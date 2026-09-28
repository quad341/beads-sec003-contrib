package main

import "testing"

func TestMutationKindFor_MergeOverridesRowDiff(t *testing.T) {
	for _, kind := range []RowDiffKind{RowDiffAdded, RowDiffRemoved, RowDiffModified, RowDiffDepAdded, RowDiffDepRemoved} {
		got := MutationKindFor(kind, true)
		if got != "merge" {
			t.Errorf("MutationKindFor(%v, isMergeCommit=true) = %q, want merge", kind, got)
		}
	}
}

func TestMutationKindFor_NonMergeMapping(t *testing.T) {
	cases := []struct {
		kind RowDiffKind
		want string
	}{
		{RowDiffAdded, "create"},
		{RowDiffRemoved, "delete"},
		{RowDiffModified, "update"},
		{RowDiffDepAdded, "dep_add"},
		{RowDiffDepRemoved, "dep_remove"},
	}
	for _, c := range cases {
		got := MutationKindFor(c.kind, false)
		if got != c.want {
			t.Errorf("MutationKindFor(%v, false) = %q, want %q", c.kind, got, c.want)
		}
	}
}
