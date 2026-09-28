package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCanarySchedule_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schedule.json")

	want := CanarySchedule{
		ScheduleID: "sched-1",
		CloneID:    "clone-1",
		Cadence:    "nightly",
		LastRunID:  "run-41",
		NextDueAt:  time.Date(2026, 9, 29, 3, 0, 0, 0, time.UTC),
	}

	if err := SaveSchedule(path, want); err != nil {
		t.Fatalf("SaveSchedule: %v", err)
	}
	got, err := LoadSchedule(path)
	if err != nil {
		t.Fatalf("LoadSchedule: %v", err)
	}

	if got.ScheduleID != want.ScheduleID || got.CloneID != want.CloneID ||
		got.Cadence != want.Cadence || got.LastRunID != want.LastRunID {
		t.Errorf("round trip field mismatch: got %+v, want %+v", got, want)
	}
	if !got.NextDueAt.Equal(want.NextDueAt) {
		t.Errorf("NextDueAt round trip = %v, want %v", got.NextDueAt, want.NextDueAt)
	}
}

func TestLoadSchedule_MissingFile(t *testing.T) {
	_, err := LoadSchedule(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected error loading a missing schedule file")
	}
}

func TestCanarySchedule_Due(t *testing.T) {
	now := time.Date(2026, 9, 28, 3, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		nextDueAt time.Time
		want      bool
	}{
		{"past due", now.Add(-time.Hour), true},
		{"exactly due", now, true},
		{"not yet due", now.Add(time.Hour), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := CanarySchedule{NextDueAt: tt.nextDueAt}
			if got := s.Due(now); got != tt.want {
				t.Errorf("Due() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanarySchedule_Advance(t *testing.T) {
	s := CanarySchedule{
		ScheduleID: "sched-1",
		CloneID:    "clone-1",
		Cadence:    "nightly",
		LastRunID:  "run-40",
		NextDueAt:  time.Date(2026, 9, 28, 3, 0, 0, 0, time.UTC),
	}
	next := time.Date(2026, 9, 29, 3, 0, 0, 0, time.UTC)

	advanced := s.Advance("run-41", next)

	if advanced.LastRunID != "run-41" {
		t.Errorf("LastRunID = %q, want run-41", advanced.LastRunID)
	}
	if !advanced.NextDueAt.Equal(next) {
		t.Errorf("NextDueAt = %v, want %v", advanced.NextDueAt, next)
	}
	if advanced.ScheduleID != s.ScheduleID || advanced.CloneID != s.CloneID || advanced.Cadence != s.Cadence {
		t.Error("Advance must preserve identity fields")
	}
	if s.LastRunID != "run-40" {
		t.Error("Advance must not mutate the receiver")
	}
}

func TestNextDueAfter_Nightly(t *testing.T) {
	from := time.Date(2026, 9, 28, 3, 0, 0, 0, time.UTC)
	got, err := NextDueAfter("nightly", from)
	if err != nil {
		t.Fatalf("NextDueAfter: %v", err)
	}
	want := from.Add(24 * time.Hour)
	if !got.Equal(want) {
		t.Errorf("NextDueAfter(nightly) = %v, want %v", got, want)
	}
}

func TestNextDueAfter_UnknownCadence(t *testing.T) {
	if _, err := NextDueAfter("hourly", time.Now()); err == nil {
		t.Fatal("expected error for unknown cadence")
	}
}
