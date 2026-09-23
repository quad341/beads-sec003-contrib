package doctor

import (
	"context"
	"fmt"
	"strings"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/storage/schema"
)

// CheckAuxRowIDsWithStore reports events/comments/issue_snapshots/
// compaction_snapshots rows whose id has drifted from its content-derived
// target (internal/storage/rowid) — a clone that ran the 0037/bd-ri8bd
// backfills independently of its peers, or whose backfill was interrupted
// mid-pass (bd-578h9). Unversioned newest-wins replication (Protocol v0.1
// §C) only converges rows whose ids are functions of their content, so a
// drifted row's clones never merge into one (be-uqzw5).
func CheckAuxRowIDsWithStore(ss *SharedStore) DoctorCheck {
	store := ss.Store()
	if store == nil {
		return DoctorCheck{
			Name:    "Aux Row IDs",
			Status:  StatusOK,
			Message: "No database yet",
		}
	}
	return checkAuxRowIDsWithStore(store)
}

func checkAuxRowIDsWithStore(store *dolt.DoltStore) DoctorCheck {
	anomalies, err := schema.ScanAuxRowIDs(context.Background(), store.UnderlyingDB())
	if err != nil {
		return DoctorCheck{
			Name:    "Aux Row IDs",
			Status:  StatusWarning,
			Message: "Unable to scan aux row ids",
			Detail:  err.Error(),
		}
	}
	if len(anomalies) == 0 {
		return DoctorCheck{
			Name:    "Aux Row IDs",
			Status:  StatusOK,
			Message: "All aux row ids content-derived",
		}
	}

	var parts []string
	var total int
	for _, a := range anomalies {
		total += a.Count
		parts = append(parts, fmt.Sprintf("%s: %d row(s)", a.Table, a.Count))
	}

	return DoctorCheck{
		Name:    "Aux Row IDs",
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d aux row(s) with drifted ids — divergent ids merge as duplicates under newest-wins replication", total),
		Detail:  strings.Join(parts, "; "),
		Fix:     "Run: bd doctor --fix (re-keys drifted rows to their content-derived id)",
	}
}
