package main

// RowDiffKind classifies how one issue's row (or its dependency edges)
// changed between two commits, as discovered by DiscoverTouchedIssues.
type RowDiffKind int

const (
	RowDiffAdded RowDiffKind = iota
	RowDiffRemoved
	RowDiffModified
	RowDiffDepAdded
	RowDiffDepRemoved
)

// MutationKindFor maps a local RowDiffKind to the ERD's mutation_kind
// vocabulary (be-hs42e.5 §5: "create" | "update" | "delete" | "dep_add" |
// "dep_remove" | "merge"). isMergeCommit overrides to "merge" regardless of
// the row-diff kind, since merge-shape is a property of commit parentage
// (2+ parents), not of what changed within the row.
func MutationKindFor(kind RowDiffKind, isMergeCommit bool) string {
	if isMergeCommit {
		return "merge"
	}
	switch kind {
	case RowDiffAdded:
		return "create"
	case RowDiffRemoved:
		return "delete"
	case RowDiffModified:
		return "update"
	case RowDiffDepAdded:
		return "dep_add"
	case RowDiffDepRemoved:
		return "dep_remove"
	default:
		return "unknown"
	}
}
