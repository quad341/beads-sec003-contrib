// Package main implements the Phase 4 oracle-query layer (be-3j80a): a
// read-only Dolt AS OF snapshot reader for the replay-with-oracle harness,
// consumed by the comparator sibling (be-f2gog).
//
// Given a corpus clone directory (scripts/corpus-acquire's output) plus a
// commit hash and an issue id, it returns the issue's row as it stood at
// that commit as a plain JSON object, ready for the comparator's JCS
// canonicalization without translation.
//
// It never opens a write-capable connection and never touches the shared
// production Dolt server on :28231 (NFR1/NFR2) — only the local restored
// clone directory, queried via `dolt sql -q ... AS OF <ref> ...` (never `bd
// sql`, which refuses outright in embedded mode — see be-hs42e.6.2).
//
// Usage:
//
//	go run ./scripts/oracle-query -dir=<clone-dir> -ref=<commit-hash> -issue=<issue-id>
package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/beads/internal/storage/issueops"
)

func main() {
	dir := flag.String("dir", "", "corpus clone directory produced by scripts/corpus-acquire (required)")
	ref := flag.String("ref", "", "commit hash to query the issue's row AS OF (required)")
	issue := flag.String("issue", "", "issue id to query (required)")
	flag.Parse()

	if *dir == "" || *ref == "" || *issue == "" {
		fmt.Fprintln(os.Stderr, "usage: oracle-query -dir=<clone-dir> -ref=<commit-hash> -issue=<issue-id>")
		os.Exit(2)
	}

	row, err := QueryAsOf(context.Background(), *dir, *ref, *issue)
	if err != nil {
		fmt.Fprintln(os.Stderr, "oracle-query:", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(row); err != nil {
		fmt.Fprintln(os.Stderr, "oracle-query: encoding result:", err)
		os.Exit(1)
	}
}

// QueryAsOf returns the row for issueID in the issues table of the Dolt
// database at dir, as it stood at commit ref, or nil if no such row existed
// at that commit (not yet created, or since deleted). dir must be a local,
// unserved clone directory — see servedDataDir.
//
// ref and issueID are spliced directly into the `dolt sql -q` argument
// (dolt's AS OF clause and CLI have no bind-parameter API), so both are
// charset-validated first via issueops.ValidateRef.
func QueryAsOf(ctx context.Context, dir, ref, issueID string) (map[string]string, error) {
	if err := issueops.ValidateRef(ref); err != nil {
		return nil, fmt.Errorf("oracle-query: invalid ref %q: %w", ref, err)
	}
	if err := issueops.ValidateRef(issueID); err != nil {
		return nil, fmt.Errorf("oracle-query: invalid issue id %q: %w", issueID, err)
	}
	if served := servedDataDir(dir); served != "" {
		return nil, fmt.Errorf("oracle-query: %s is under %s, which a dolt sql-server is serving; querying it would route through that server instead of the local clone", dir, served)
	}

	query := fmt.Sprintf("SELECT * FROM issues AS OF '%s' WHERE id = '%s'", ref, issueID) //nolint:gosec // G201 -- dolt sql -q has no bind-parameter API; ref/issueID are charset-validated above via issueops.ValidateRef
	out, err := doltRun(ctx, dir, "sql", "-q", query, "-r", "csv")
	if err != nil {
		return nil, fmt.Errorf("oracle-query: querying %s AS OF %s: %w", issueID, ref, err)
	}

	r := csv.NewReader(bytes.NewReader(out))
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("oracle-query: parsing CSV from query %q: %w", query, err)
	}
	if len(rows) < 2 {
		return nil, nil
	}
	header, data := rows[0], rows[1]
	result := make(map[string]string, len(header))
	for i, col := range header {
		if i < len(data) {
			result[col] = data[i]
		}
	}
	return result, nil
}

// servedDataDir returns dir or its nearest ancestor that a dolt sql-server is
// serving, identified by the <dir>/.dolt/sql-server.info record the server
// writes at startup, or "" when there is none. The dolt CLI routes commands
// for any database under such a directory through that server.
func servedDataDir(dir string) string {
	dir = filepath.Clean(dir)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".dolt", "sql-server.info")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// sanitizedEnv returns base with every environment variable this tool must
// never blindly trust stripped out (mirrors scripts/corpus-acquire's
// pattern): the ambient Dolt server address/port and actor identity, none
// of which this tool's own read-only query needs, and all of which have
// caused a real incident when leaked into a child process (cairn
// beads-testdb-production-leak: ambient BEADS_DOLT_SERVER_PORT fail-open
// created a scratch database directly on the shared production server).
func sanitizedEnv(base []string) []string {
	strip := map[string]bool{
		"BEADS_DOLT_SERVER_PORT": true,
		"BEADS_DOLT_PORT":        true,
		"BEADS_ACTOR":            true,
		"BD_ACTOR":               true,
		"GT_ROOT":                true,
	}
	out := make([]string, 0, len(base))
	for _, kv := range base {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if strip[name] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// doltRun runs the dolt CLI (resolved via PATH at call time, like any other
// exec.Command — this is also what lets a test's PATH-prepended shim
// intercept it) with dir as its working directory and a sanitized child
// environment. It never invokes git and never receives a non-file:// URL or
// server address from any caller in this package.
func doltRun(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "dolt", args...)
	cmd.Dir = dir
	cmd.Env = sanitizedEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("dolt %s (dir=%s): %w\n%s", strings.Join(args, " "), dir, err, out)
	}
	return out, nil
}
