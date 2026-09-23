package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/storage"
)

type fakeVersionLister struct {
	storage.DoltStorage
	called   bool
	versions []storage.IssueVersion
}

func (f *fakeVersionLister) ListVersions(_ context.Context, _ string) ([]storage.IssueVersion, error) {
	f.called = true
	return f.versions, nil
}

type notAVersionLister struct{ storage.DoltStorage }

// TestVersionsOffButRecordedStillLists is finding 3 from bee's #6661 review:
// a store that recorded for a month and was then switched off HAS versions,
// and refusing with "nothing is recorded" states a fact about this
// invocation's config as if it were a fact about the store.
func TestVersionsOffButRecordedStillLists(t *testing.T) {
	backend := &fakeVersionLister{versions: []storage.IssueVersion{{Revision: 3}, {Revision: 2}}}

	got, err := runVersions(context.Background(), backend, "bd-1", false, true)

	if err != nil {
		t.Fatalf("runVersions(recording=false, 2 rows) = %v, want the rows", err)
	}
	if len(got.Versions) != 2 {
		t.Fatalf("got %d versions, want 2 — recorded history must survive the switch going off", len(got.Versions))
	}
	if got.Recording {
		t.Error("Recording should be false so the caller can say the listing ends where recording stopped")
	}
}

// TestVersionsOffAndEmptyRefusesWithRemedy is the empty arm of the pair above,
// and together they are the whole of finding 3: recording off decides the
// FOOTER, never whether to read. Off with rows recorded lists them; off with
// nothing recorded is the one case that refuses, and it refuses with the
// remedy rather than returning an empty list.
//
// That distinction is what #5898 exists to make. An empty list is a claim
// about the bead ("it has no versions"); the truth here is a claim about the
// store ("it never recorded any"). Returning the former for the latter is the
// wrong-answer-not-an-empty-one failure.
//
// This was TestVersionsRefusesWhenFlagOff, which asserted that the read was
// refused BEFORE the backend was consulted. Finding 3 removed that behaviour
// deliberately, so the assertion went with it -- but the fixture it had been
// asserting against stayed behind, built, unused and ending in `_ = backend`,
// under a name that still promised the old contract. Renamed to what it
// actually pins, and the dead fixture is gone.
// (bee-ghosttrack, #6661 third review, finding 3.)
func TestVersionsOffAndEmptyRefusesWithRemedy(t *testing.T) {
	got, err := runVersions(context.Background(), &fakeVersionLister{}, "bd-1", false, true)

	if !errors.Is(err, errVersionedHistoryOff) {
		t.Fatalf("runVersions(recording=false, empty) error = %v, want errVersionedHistoryOff", err)
	}
	if len(got.Versions) != 0 {
		t.Errorf("returned %d versions, want none", len(got.Versions))
	}
}

// TestVersionsUnresolvedIDIsNotAnEmptyBead is finding 4: falling through on an
// unresolved id is right (a deleted bead can still have versions), but only
// until the answer is empty. Then it is "no such bead", not "none yet".
func TestVersionsUnresolvedIDIsNotAnEmptyBead(t *testing.T) {
	_, err := runVersions(context.Background(), &fakeVersionLister{}, "nosuch-id", true, false)

	if !errors.Is(err, errNoSuchBead) {
		t.Fatalf("runVersions(resolved=false, empty) error = %v, want errNoSuchBead", err)
	}
}

// TestVersionsRefusesUnsupportedBackend pins that "this backend cannot answer"
// is distinguishable from "there are no versions" -- the third shape of an
// answer, not a smaller success.
func TestVersionsRefusesUnsupportedBackend(t *testing.T) {
	_, err := runVersions(context.Background(), &notAVersionLister{}, "bd-1", true, true)

	if !errors.Is(err, errVersionsUnsupported) {
		t.Fatalf("runVersions(unsupported backend) error = %v, want errVersionsUnsupported", err)
	}
	if errors.Is(err, errVersionedHistoryOff) {
		t.Error("an unsupported backend must not be reported as the feature being off: different fixes")
	}
}

// TestVersionsEmptyIsNotAnError pins the other side: a supported, enabled
// store with nothing recorded returns an empty list and no error. That is a
// truthful answer, and the command renders the no-backfill explanation.
func TestVersionsEmptyIsNotAnError(t *testing.T) {
	got, err := runVersions(context.Background(), &fakeVersionLister{}, "bd-1", true, true)
	if err != nil {
		t.Fatalf("runVersions on an empty store = %v, want nil", err)
	}
	if len(got.Versions) != 0 {
		t.Errorf("got %d versions, want 0", len(got.Versions))
	}
}

// TestVersionListerPeelsDecorators is the regression guard for the bug this
// command shipped with first: cmd/bd holds the DECORATED store, VersionLister
// is an optional capability none of the decorators forwards, and asserting
// against the wrapper reported "unsupported" for a store that could serve
// versions. The capability must be found through the decorator chain.
func TestVersionListerPeelsDecorators(t *testing.T) {
	inner := &fakeVersionLister{versions: []storage.IssueVersion{{Revision: 7}}}
	decorated := storage.NewHookFiringStore(inner, nil)

	lister, ok := versionListerFor(decorated)
	if !ok {
		t.Fatal("versionListerFor could not see through the decorator chain")
	}
	got, err := lister.ListVersions(context.Background(), "bd-1")
	if err != nil {
		t.Fatalf("ListVersions through decorator = %v", err)
	}
	if len(got) != 1 || got[0].Revision != 7 {
		t.Errorf("got %+v, want the inner store's revision 7", got)
	}
}

// TestRestrictionLabelsAreDistinct pins that the four removal reasons render
// as four different sentences. Collapsing them into one "gone" would erase
// the distinction R10 requires, and in particular "unknown" (this store has
// no lineage knowledge) is NOT "gone" -- the conformance suite pins that a
// never-synced store answers Unknown rather than Gone.
func TestRestrictionLabelsAreDistinct(t *testing.T) {
	seen := map[string]string{}
	for _, r := range []string{"gone_retention", "gone_erasure", "gone_reorganization", "unknown"} {
		label := restrictionLabel(r)
		if label == "" {
			t.Errorf("restrictionLabel(%q) is empty", r)
		}
		if prev, dup := seen[label]; dup {
			t.Errorf("restrictionLabel(%q) and (%q) both render %q; they are different answers", r, prev, label)
		}
		seen[label] = r
	}
	if restrictionLabel("unknown") == restrictionLabel("gone_erasure") {
		t.Error("unknown must not read as gone: no lineage knowledge is not erasure")
	}
}

// TestVersionsJSONDoesNotCollideWithIssueRevision pins that the versions
// payload does not emit a "revision" key.
//
// types.Issue already ships `json:"revision"` and it is the row-lock CAS
// token -- an unrelated value, and a STRING where this one is a number. One
// key carrying two meanings, in the place machines read, is the `bd version`
// collision again with worse consequences: a script that learned one shape
// gets the other and cannot tell. Raised by bee-ghosttrack's consumer read on
// #5898, who reported a couple of hundred script files parsing bd --json.
func TestVersionsJSONDoesNotCollideWithIssueRevision(t *testing.T) {
	blob, err := json.Marshal(storage.IssueVersion{Revision: 7, IssueID: "bd-1"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var generic map[string]any
	if err := json.Unmarshal(blob, &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, collides := generic["revision"]; collides {
		t.Errorf(`IssueVersion emits a "revision" key; types.Issue already uses that key for the row-lock token. Got: %s`, blob)
	}
	got, ok := generic["local_revision"]
	if !ok {
		t.Fatalf(`IssueVersion has no "local_revision" key. Got: %s`, blob)
	}
	if n, isNum := got.(float64); !isNum || n != 7 {
		t.Errorf("local_revision = %v, want 7", got)
	}
}

// --- be-hs42e.9: distinct exit codes, and a normalized restriction enum in
// --json, so callers branch on codes, not on rendered text (bee-ghosttrack's
// review on #5898, referencing the already-landed #6661). ---

// TestVersionsExitCodesAreStableAndDistinct pins the four ExitVersions*
// literals. They are script-facing -- the bead's whole point is that a
// caller branches on the number instead of parsing English -- so an
// accidental renumbering is a breaking change the same way it would be for
// sync.go's ExitSyncConflict (see sync_test.go). Distinctness matters as
// much as the values themselves: two refusal classes sharing one code would
// silently recreate the "can't tell them apart" problem this bead exists to
// fix.
func TestVersionsExitCodesAreStableAndDistinct(t *testing.T) {
	want := map[string]int{
		"ExitVersionsFeatureOff":  20,
		"ExitVersionsNotFound":    21,
		"ExitVersionsUnsupported": 22,
		"ExitVersionsGone":        23,
	}
	got := map[string]int{
		"ExitVersionsFeatureOff":  ExitVersionsFeatureOff,
		"ExitVersionsNotFound":    ExitVersionsNotFound,
		"ExitVersionsUnsupported": ExitVersionsUnsupported,
		"ExitVersionsGone":        ExitVersionsGone,
	}
	seen := map[int]string{}
	for name, code := range got {
		if code != want[name] {
			t.Errorf("%s = %d, want %d (external scripts branch on this literal)", name, code, want[name])
		}
		if prev, dup := seen[code]; dup {
			t.Errorf("%s and %s share exit code %d; a script could not tell them apart", name, prev, code)
		}
		seen[code] = name
	}
}

// TestVersionsExitErrorMapsSentinelsToDistinctCodes is versionsExitError --
// the RunE switch pulled out so it can be exercised directly against the
// sentinel errors runVersions already returns, without a store that has to
// survive ResolvePartialID's own SearchIssues call. errors.Is must see
// through a wrapped sentinel the same way the original inline switch did.
func TestVersionsExitErrorMapsSentinelsToDistinctCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"not found", errNoSuchBead, ExitVersionsNotFound},
		{"feature off", errVersionedHistoryOff, ExitVersionsFeatureOff},
		{"unsupported backend", errVersionsUnsupported, ExitVersionsUnsupported},
		{"wrapped not found", fmt.Errorf("resolve: %w", errNoSuchBead), ExitVersionsNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handled := versionsExitError(tt.err, "bd-1")
			if handled == nil {
				t.Fatalf("versionsExitError(%v) = nil, want a refusal", tt.err)
			}
			code, ok := exitCodeFromError(handled)
			if !ok {
				t.Fatalf("versionsExitError(%v) did not return an *exitError", tt.err)
			}
			if code != tt.want {
				t.Errorf("versionsExitError(%v) exit code = %d, want %d", tt.err, code, tt.want)
			}
		})
	}
}

// TestVersionsExitErrorGenericFailureKeepsCodeOne guards against an
// over-eager mapping: an error that is none of the three named classes must
// still exit 1, not claim one of the new dedicated codes for itself.
func TestVersionsExitErrorGenericFailureKeepsCodeOne(t *testing.T) {
	handled := versionsExitError(errors.New("boom"), "bd-1")
	if handled == nil {
		t.Fatal("versionsExitError(generic error) = nil, want a refusal")
	}
	code, ok := exitCodeFromError(handled)
	if !ok {
		t.Fatalf("versionsExitError(generic error) did not return an *exitError")
	}
	if code != 1 {
		t.Errorf("versionsExitError(generic error) exit code = %d, want 1 (only the three named classes get a dedicated code)", code)
	}
}

// TestVersionsExitErrorNilIsNil pins the success path through the extracted
// function: no error in means no refusal out, so RunE's caller can tell
// "nothing to handle" from "handled, stop here" with a single nil check.
func TestVersionsExitErrorNilIsNil(t *testing.T) {
	if handled := versionsExitError(nil, "bd-1"); handled != nil {
		t.Errorf("versionsExitError(nil) = %v, want nil", handled)
	}
}

// TestVersionsNormalizesEmptyRestrictionToUnknown is the JSON half of the
// bead. ListVersionsInTx's COALESCE(removed_restriction, ”) reads a legacy
// or unset row back as "", and that raw "" was flowing straight into --json
// output unnormalized while the text renderer (restrictionLabel) already
// treated "" and "unknown" as the same answer -- an inconsistency between
// the two front ends of one command. "unknown" means this store has no
// lineage knowledge; it is a specific, machine-readable answer, not a
// stand-in for silence, so the empty string must not survive into the wire
// format unlabelled. Mirrors the normalization issueops.AsOfReadInTx already
// applies to this same column (asof_read.go) -- same value, not a shared
// type, since storage.IssueVersion cannot import issueops.AsOfRestriction
// without a cycle (issueops imports storage).
func TestVersionsNormalizesEmptyRestrictionToUnknown(t *testing.T) {
	removedAt := time.Now()
	backend := &fakeVersionLister{versions: []storage.IssueVersion{
		{Revision: 5, RemovedAt: &removedAt, RemovedRestriction: ""},
	}}

	got, err := runVersions(context.Background(), backend, "bd-1", true, true)
	if err != nil {
		t.Fatalf("runVersions = %v, want nil", err)
	}
	if len(got.Versions) != 1 {
		t.Fatalf("got %d versions, want 1", len(got.Versions))
	}
	if got.Versions[0].RemovedRestriction != "unknown" {
		t.Errorf(`RemovedRestriction = %q, want "unknown" (empty must normalize the same way issueops.AsOfReadInTx does)`, got.Versions[0].RemovedRestriction)
	}
}

// TestVersionsPreservesNonEmptyRestriction guards the normalization from
// overwriting a real answer: a row that already names a restriction must
// pass through unchanged.
func TestVersionsPreservesNonEmptyRestriction(t *testing.T) {
	removedAt := time.Now()
	backend := &fakeVersionLister{versions: []storage.IssueVersion{
		{Revision: 5, RemovedAt: &removedAt, RemovedRestriction: "gone_retention"},
	}}

	got, err := runVersions(context.Background(), backend, "bd-1", true, true)
	if err != nil {
		t.Fatalf("runVersions = %v, want nil", err)
	}
	if got.Versions[0].RemovedRestriction != "gone_retention" {
		t.Errorf("RemovedRestriction = %q, want unchanged %q", got.Versions[0].RemovedRestriction, "gone_retention")
	}
}

// TestVersionsDoesNotTouchRestrictionOnALiveVersion guards the normalization
// from writing into rows that were never removed: RemovedRestriction only
// means something once Removed() is true (RemovedAt != nil). On a live row
// the empty string is simply the field's zero value, not an unknown
// restriction, and normalizing it would fabricate a removal that never
// happened.
func TestVersionsDoesNotTouchRestrictionOnALiveVersion(t *testing.T) {
	backend := &fakeVersionLister{versions: []storage.IssueVersion{
		{Revision: 5, RemovedRestriction: ""},
	}}

	got, err := runVersions(context.Background(), backend, "bd-1", true, true)
	if err != nil {
		t.Fatalf("runVersions = %v, want nil", err)
	}
	if got.Versions[0].RemovedRestriction != "" {
		t.Errorf("RemovedRestriction = %q on a live version, want untouched %q", got.Versions[0].RemovedRestriction, "")
	}
}

// TestVersionsJSONRestrictionIsNeverEmptyOnARemovedRow pins the actual wire
// format end to end, same style as TestVersionsJSONDoesNotCollideWithIssueRevision
// above: a script reading --json output for a removed row must see
// "removed_restriction":"unknown", never "".
func TestVersionsJSONRestrictionIsNeverEmptyOnARemovedRow(t *testing.T) {
	removedAt := time.Now()
	backend := &fakeVersionLister{versions: []storage.IssueVersion{
		{Revision: 5, RemovedAt: &removedAt, RemovedRestriction: ""},
	}}

	got, err := runVersions(context.Background(), backend, "bd-1", true, true)
	if err != nil {
		t.Fatalf("runVersions = %v, want nil", err)
	}

	blob, err := json.Marshal(got.Versions)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var generic []map[string]any
	if err := json.Unmarshal(blob, &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(generic) != 1 {
		t.Fatalf("got %d rows, want 1", len(generic))
	}
	if r := generic[0]["removed_restriction"]; r != "unknown" {
		t.Errorf(`removed_restriction = %v, want "unknown" -- a never-synced/legacy row must not read as the empty string in --json. Got: %s`, r, blob)
	}
}
