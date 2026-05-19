package types_test

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

// roundTripGob encodes v to gob bytes, decodes into a new zero value,
// and returns the decoded value.
func roundTripGob[T any](t *testing.T, v T) T {
	t.Helper()
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		t.Fatalf("gob.Encode(%T): %v", v, err)
	}
	var decoded T
	if err := gob.NewDecoder(&buf).Decode(&decoded); err != nil {
		t.Fatalf("gob.Decode(%T): %v", v, err)
	}
	return decoded
}

func nowPtr() *time.Time {
	t := time.Now().UTC().Truncate(time.Second)
	return &t
}

func strPtr2(s string) *string { return &s }
func intPtr2(n int) *int       { return &n }

// TestGobRoundTrip_Issue verifies types.Issue encodes and decodes cleanly.
func TestGobRoundTrip_Issue(t *testing.T) {
	original := &types.Issue{
		ID:          "bd-abc123",
		Title:       "Test issue",
		Description: "A test description",
		Status:      types.StatusOpen,
		IssueType:   types.TypeTask,
		Priority:    1,
		CreatedBy:   "alice",
		Assignee:    "bob",
		CloseReason: "",
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		Labels:      []string{"label1", "label2"},
	}
	decoded := roundTripGob(t, original)
	if decoded.ID != original.ID {
		t.Errorf("ID: got %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Title != original.Title {
		t.Errorf("Title: got %q, want %q", decoded.Title, original.Title)
	}
	if decoded.Status != original.Status {
		t.Errorf("Status: got %q, want %q", decoded.Status, original.Status)
	}
	if !reflect.DeepEqual(decoded.Labels, original.Labels) {
		t.Errorf("Labels: got %v, want %v", decoded.Labels, original.Labels)
	}
}

// TestGobRoundTrip_Comment verifies types.Comment round-trips.
func TestGobRoundTrip_Comment(t *testing.T) {
	ts := time.Now().UTC().Truncate(time.Second)
	original := &types.Comment{
		ID:        "cmt-001",
		IssueID:   "bd-abc123",
		Author:    "alice",
		Text:      "A test comment",
		CreatedAt: ts,
	}
	decoded := roundTripGob(t, original)
	if decoded.ID != original.ID || decoded.Text != original.Text {
		t.Errorf("Comment round-trip: got %+v, want %+v", decoded, original)
	}
}

// TestGobRoundTrip_Event verifies types.Event round-trips.
func TestGobRoundTrip_Event(t *testing.T) {
	ts := time.Now().UTC().Truncate(time.Second)
	original := &types.Event{
		ID:        "evt-001",
		IssueID:   "bd-abc123",
		EventType: "status_change",
		Actor:     "alice",
		CreatedAt: ts,
	}
	decoded := roundTripGob(t, original)
	if decoded.ID != original.ID || decoded.EventType != original.EventType {
		t.Errorf("Event round-trip: got %+v, want %+v", decoded, original)
	}
}

// TestGobRoundTrip_Dependency verifies types.Dependency round-trips.
func TestGobRoundTrip_Dependency(t *testing.T) {
	original := &types.Dependency{
		IssueID:     "bd-abc123",
		DependsOnID: "bd-def456",
		Type:        "blocks",
	}
	decoded := roundTripGob(t, original)
	if decoded.IssueID != original.IssueID || decoded.DependsOnID != original.DependsOnID {
		t.Errorf("Dependency round-trip: got %+v, want %+v", decoded, original)
	}
}

// TestGobRoundTrip_IssueWithDependencyMetadata verifies round-trip.
func TestGobRoundTrip_IssueWithDependencyMetadata(t *testing.T) {
	original := &types.IssueWithDependencyMetadata{
		Issue:          types.Issue{ID: "bd-abc123", Title: "Test issue"},
		DependencyType: "blocks",
	}
	decoded := roundTripGob(t, original)
	if decoded.ID != original.ID {
		t.Errorf("IssueWithDependencyMetadata round-trip: got ID=%q, want %q", decoded.ID, original.ID)
	}
	if decoded.DependencyType != original.DependencyType {
		t.Errorf("IssueWithDependencyMetadata.DependencyType: got %q, want %q",
			decoded.DependencyType, original.DependencyType)
	}
}

// TestGobRoundTrip_BlockedIssue verifies types.BlockedIssue round-trips.
func TestGobRoundTrip_BlockedIssue(t *testing.T) {
	original := &types.BlockedIssue{
		Issue:          types.Issue{ID: "bd-abc123", Title: "Blocked issue"},
		BlockedByCount: 1,
		BlockedBy:      []string{"bd-def456"},
	}
	decoded := roundTripGob(t, original)
	if decoded.ID != original.ID {
		t.Errorf("BlockedIssue.ID: got %q, want %q", decoded.ID, original.ID)
	}
	if decoded.BlockedByCount != original.BlockedByCount {
		t.Errorf("BlockedIssue.BlockedByCount: got %d, want %d",
			decoded.BlockedByCount, original.BlockedByCount)
	}
}
