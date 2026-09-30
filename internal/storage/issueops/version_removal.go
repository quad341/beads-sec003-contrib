package issueops

import (
	"context"
	"errors"
)

// issue_versions.removed_at / removed_reason (migration 0067, widened by
// 0069) are the soft-removal marker for one minted version: a removed row
// stays in the table as durable evidence that the store once held that
// version and no longer serves it, which is what lets a resolver answer
// "gone" rather than "unknown" (backend/conformance R20-l/R20-n). This file
// is the SQL half of writing that marker.
//
// VersionRemovalReasonXxx spell the same vocabulary as
// backend/conformance.RemovalReason.String() (retention / erasure /
// reorganization), duplicated here rather than imported for the same reason
// EpochBumpReasonXxx is (store_epoch.go): production code must not depend on
// a test-support package. TestVersionRemovalReasonsMatchConformance (in the
// dolt leg, which can see both packages) pins the two spellings together.
const (
	VersionRemovalReasonRetention      = "retention"
	VersionRemovalReasonErasure        = "erasure"
	VersionRemovalReasonReorganization = "reorganization"
)

// ErrVersionNotFound is returned by RemoveVersionInTx when issue_versions has
// no row for the named (issue, revision).
var ErrVersionNotFound = errors.New("issue version not found")

// RemoveVersionInTx soft-removes one minted version: it stamps removed_at and
// removed_reason on the (issueID, revision) row and never deletes it.
//
// The first removal wins. Removing a version that is already removed is a
// no-op that returns nil and leaves the recorded time and reason untouched,
// so a later, different reason can never rewrite why the version went away.
// A reason outside the VersionRemovalReasonXxx vocabulary, or a
// (issueID, revision) with no row, is an error and writes nothing.
//
// Like BumpEpochInTx this is only the SQL half: it neither stages nor commits
// the issue_versions table, and callers that publish through Dolt must commit
// this transaction first and stage the working set as a separate later step.
func RemoveVersionInTx(ctx context.Context, tx DBTX, issueID string, revision int64, reason string) error {
	// RED stub (mol-tdd-build be-5qmyx): the behavior above is what the tests
	// specify; GREEN replaces this body.
	return errors.New("issueops: RemoveVersionInTx not implemented")
}
