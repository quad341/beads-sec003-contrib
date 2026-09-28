package main

import (
	"context"
	"testing"
)

func TestListCommits_OldestFirst(t *testing.T) {
	dir := initBdProject(t, "src")
	data := dataDir(t, dir)

	out := runBd(t, dir, "create", "First", "--type", "task", "--json")
	id1 := jsonID(t, out)
	runBd(t, dir, "update", id1, "--description", "second change")

	commits, err := ListCommits(context.Background(), data)
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	if len(commits) < 2 {
		t.Fatalf("expected at least 2 commits (init + create + update), got %d: %+v", len(commits), commits)
	}
	seen := map[string]bool{}
	for _, c := range commits {
		if seen[c.Hash] {
			t.Fatalf("duplicate commit hash in ListCommits result: %+v", commits)
		}
		seen[c.Hash] = true
	}
}

func TestDiscoverTouchedIssues_CreateIsAdded(t *testing.T) {
	dir := initBdProject(t, "src")
	data := dataDir(t, dir)
	from := headCommit(t, data)
	out := runBd(t, dir, "create", "Widget", "--type", "task", "--json")
	id := jsonID(t, out)
	to := headCommit(t, data)

	touched, err := DiscoverTouchedIssues(context.Background(), data, from, to)
	if err != nil {
		t.Fatalf("DiscoverTouchedIssues: %v", err)
	}
	if len(touched) != 1 || touched[0].IssueID != id || touched[0].Diff != RowDiffAdded {
		t.Fatalf("DiscoverTouchedIssues = %+v, want exactly one RowDiffAdded for %s", touched, id)
	}
}

func TestDiscoverTouchedIssues_UpdateIsModified(t *testing.T) {
	dir := initBdProject(t, "src")
	data := dataDir(t, dir)
	out := runBd(t, dir, "create", "Widget", "--type", "task", "--json")
	id := jsonID(t, out)

	from := headCommit(t, data)
	runBd(t, dir, "update", id, "--description", "changed")
	to := headCommit(t, data)

	touched, err := DiscoverTouchedIssues(context.Background(), data, from, to)
	if err != nil {
		t.Fatalf("DiscoverTouchedIssues: %v", err)
	}
	found := false
	for _, ti := range touched {
		if ti.IssueID == id && ti.Diff == RowDiffModified {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a RowDiffModified entry for %s, got %+v", id, touched)
	}
}

func TestDiscoverTouchedIssues_DepAddIsDepAdded(t *testing.T) {
	dir := initBdProject(t, "src")
	data := dataDir(t, dir)
	outA := runBd(t, dir, "create", "A", "--type", "task", "--json")
	idA := jsonID(t, outA)
	outB := runBd(t, dir, "create", "B", "--type", "task", "--json")
	idB := jsonID(t, outB)

	from := headCommit(t, data)
	runBd(t, dir, "dep", "add", idA, idB)
	to := headCommit(t, data)

	touched, err := DiscoverTouchedIssues(context.Background(), data, from, to)
	if err != nil {
		t.Fatalf("DiscoverTouchedIssues: %v", err)
	}
	foundDepKind := false
	for _, ti := range touched {
		if ti.IssueID == idA && ti.Diff == RowDiffDepAdded {
			foundDepKind = true
		}
	}
	if !foundDepKind {
		t.Fatalf("expected a RowDiffDepAdded entry for %s, got %+v", idA, touched)
	}
}

func TestListCommits_DetectsMergeCommit(t *testing.T) {
	dir := initBdProject(t, "src")
	data := dataDir(t, dir)

	runBd(t, dir, "create", "Base", "--type", "task", "--json")
	runDolt(t, data, "checkout", "-b", "feature")
	runBd(t, dir, "create", "Feature Work", "--type", "task", "--json")
	runDolt(t, data, "checkout", "main")
	runDolt(t, data, "merge", "feature", "-m", "merge feature into main")

	commits, err := ListCommits(context.Background(), data)
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	sawMerge := false
	for _, c := range commits {
		if c.IsMerge {
			sawMerge = true
		}
	}
	if !sawMerge {
		t.Fatalf("expected at least one merge commit among %+v", commits)
	}
}
