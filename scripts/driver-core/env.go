package main

import "strings"

// deniedEnvVars are ambient environment variables that must never reach a
// dolt/bd subprocess driver-core spawns (NFR1/NFR2/NFR3): letting one through
// risks silently routing a replay at a shared/production store, or attaching
// the wrong actor identity to a mutation this harness makes on its own
// behalf. Union of oracle-query's (be-3j80a) and mutation-translator's
// (be-2cp1d) strip-lists -- driver-core spawns both, so it must strip both.
var deniedEnvVars = map[string]bool{
	"BEADS_DOLT_SERVER_PORT":      true,
	"BEADS_DOLT_PORT":             true,
	"BEADS_ACTOR":                 true,
	"BD_ACTOR":                    true,
	"GT_ROOT":                     true,
	"BEADS_DIR":                   true,
	"BEADS_HOLDER_TOKEN":          true,
	"GC_BEADS_SCOPE_ROOT":         true,
	"BEADS_DOLT_AUTO_START":       true,
	"BEADS_DOLT_SYNC_CLI_REMOTES": true,
	"BEADS_BACKUP_ENABLED":        true,
}

// sanitizedEnv returns base with every denied ambient var removed, for use
// as the Env of any dolt/bd subprocess driver-core spawns.
func sanitizedEnv(base []string) []string {
	out := make([]string, 0, len(base))
	for _, kv := range base {
		name, _, _ := strings.Cut(kv, "=")
		if deniedEnvVars[name] {
			continue
		}
		out = append(out, kv)
	}
	return out
}
