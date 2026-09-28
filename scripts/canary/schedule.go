package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// CanarySchedule is this package's own cadence record (be-hs42e.5.6 design
// decision 3: canary-ness is tracked at this orchestrator layer, not by
// extending driver-core's REPLAY_RUN.mode).
type CanarySchedule struct {
	ScheduleID string    `json:"schedule_id"`
	CloneID    string    `json:"clone_id"`
	Cadence    string    `json:"cadence"`
	LastRunID  string    `json:"last_run_id"`
	NextDueAt  time.Time `json:"next_due_at"`
}

// SaveSchedule writes s to path as JSON, overwriting any existing file.
func SaveSchedule(path string, s CanarySchedule) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling canary schedule: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing canary schedule %s: %w", path, err)
	}
	return nil
}

// LoadSchedule reads a CanarySchedule previously written by SaveSchedule.
func LoadSchedule(path string) (CanarySchedule, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is always this process's own schedule file
	if err != nil {
		return CanarySchedule{}, fmt.Errorf("reading canary schedule %s: %w", path, err)
	}
	var s CanarySchedule
	if err := json.Unmarshal(data, &s); err != nil {
		return CanarySchedule{}, fmt.Errorf("parsing canary schedule %s: %w", path, err)
	}
	return s, nil
}

// Due reports whether the schedule's next run is due at or before now.
func (s CanarySchedule) Due(now time.Time) bool {
	return !s.NextDueAt.After(now)
}

// Advance returns a copy of s recording that runID just ran and the next
// run is due at nextDueAt. The receiver is not mutated.
func (s CanarySchedule) Advance(runID string, nextDueAt time.Time) CanarySchedule {
	s.LastRunID = runID
	s.NextDueAt = nextDueAt
	return s
}

// NextDueAfter computes the next due time for cadence, measured from from.
// "nightly" is the only cadence UC4/AF2 specify.
func NextDueAfter(cadence string, from time.Time) (time.Time, error) {
	switch cadence {
	case "nightly":
		return from.Add(24 * time.Hour), nil
	default:
		return time.Time{}, fmt.Errorf("unknown canary cadence %q", cadence)
	}
}
