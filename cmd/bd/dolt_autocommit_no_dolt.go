//go:build no_dolt

package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/steveyegge/beads/internal/storage"
)

// transact under no_dolt simply runs the function inside a storage transaction.
// The Dolt-only "explicit-commit" bookkeeping that suppresses redundant
// PersistentPostRun auto-commits is unnecessary here, since maybeAutoCommit is
// a no-op in this build. The signature mirrors the !no_dolt variant so call
// sites in shared command files compile under both build tags.
func transact(ctx context.Context, s storage.Storage, commitMsg string, fn func(tx storage.Transaction) error) error {
	return s.RunInTransaction(ctx, commitMsg, fn)
}

type doltAutoCommitMode string

const (
	doltAutoCommitOff   doltAutoCommitMode = "off"
	doltAutoCommitOn    doltAutoCommitMode = "on"
	doltAutoCommitBatch doltAutoCommitMode = "batch"
)

type doltAutoCommitParams struct {
	Command         string
	IssueIDs        []string
	MessageOverride string
}

// getDoltAutoCommitMode is hard-coded to Off under no_dolt — there is no Dolt
// commit graph to write to. Callers that branch on the mode see Off and skip
// the auto-commit work.
func getDoltAutoCommitMode() (doltAutoCommitMode, error) {
	return doltAutoCommitOff, nil
}

// maybeAutoCommit is a no-op under no_dolt — Postgres-backed projects record
// their audit trail through events/wisp_events row writes, not Dolt commits.
func maybeAutoCommit(_ context.Context, _ doltAutoCommitParams) error {
	return nil
}

// isDoltNothingToCommit returns false under no_dolt: no error path in this
// build is a "Dolt nothing to commit" sentinel, so callers should propagate
// every error they receive.
func isDoltNothingToCommit(_ error) bool {
	return false
}

// formatDoltAutoCommitMessage is preserved for the tip-metadata commit path in
// main.go that builds the message string before calling maybeAutoCommit.
// Mirrors the !no_dolt formatter so callers see identical output regardless
// of build tag.
func formatDoltAutoCommitMessage(cmd string, actor string, issueIDs []string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		cmd = "write"
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "unknown"
	}

	ids := make([]string, 0, len(issueIDs))
	seen := make(map[string]bool, len(issueIDs))
	for _, id := range issueIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	slices.Sort(ids)

	const maxIDs = 5
	if len(ids) > maxIDs {
		ids = ids[:maxIDs]
	}

	if len(ids) == 0 {
		return fmt.Sprintf("bd: %s (auto-commit) by %s", cmd, actor)
	}
	return fmt.Sprintf("bd: %s (auto-commit) by %s [%s]", cmd, actor, strings.Join(ids, ", "))
}
