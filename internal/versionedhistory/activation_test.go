package versionedhistory

import (
	"context"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
)

// unversionableStore is a store that cannot record version history: it does
// not implement storage.VersionedHistoryConfigurer. Embedding the interface
// as a nil field (rather than implementing all of storage.DoltStorage by
// hand) is the same double used by internal/eventsjournal's own activation
// tests, for the same reason: nothing here calls any method but the ones
// defined below, so the rest can panic-on-use without ever being reached.
type unversionableStore struct{ storage.DoltStorage }

func (s *unversionableStore) Close() error { return nil }

// versionableStore records what activation it was given, and answers
// GetConfig from an in-memory map so a test can drive it without a real
// Dolt-backed store.
type versionableStore struct {
	unversionableStore
	config  map[string]string
	enabled bool
	set     bool
}

func newVersionableStore(configValue string) *versionableStore {
	return &versionableStore{config: map[string]string{ConfigKey: configValue}}
}

func (s *versionableStore) GetConfig(_ context.Context, key string) (string, error) {
	return s.config[key], nil
}

func (s *versionableStore) SetVersionedHistoryEnabled(enabled bool) {
	s.enabled = enabled
	s.set = true
}

// TestConfigKeyMatchesDocumentedConvention pins the exact key an operator
// types. docs/reference/configuration.md's Project-Level Settings (Database)
// table and this constant must never drift from each other.
func TestConfigKeyMatchesDocumentedConvention(t *testing.T) {
	if ConfigKey != "versioned-history.enabled" {
		t.Errorf("ConfigKey = %q, want %q", ConfigKey, "versioned-history.enabled")
	}
}

// TestApplyIsANoOpOnAStoreThatCannotVersion pins the fail-OPEN branch. Unlike
// the events journal (whose whole point breaks silently if activation is
// dropped), a store that cannot record version history just doesn't:
// storage.go's own doc comment for VersionedHistoryConfigurer says callers
// type-assert and "a store that does not implement it simply cannot record
// version history." There is nothing to fail here.
func TestApplyIsANoOpOnAStoreThatCannotVersion(t *testing.T) {
	Apply(nil, true) // must not panic
}

// TestApplyBindsActivationToOneInstance is done-when #2 from be-41fbz, in its
// most direct form: flipping activation on one store must never reach a
// second, unrelated store instance. Every implementation stores
// versionedHistoryEnabled as an atomic.Bool on the STORE INSTANCE
// (dolt/store.go, embeddeddolt/store.go), never process-global, and this
// pins that the wiring above the atomic.Bool preserves that isolation too.
func TestApplyBindsActivationToOneInstance(t *testing.T) {
	first := &versionableStore{}
	second := &versionableStore{}

	Apply(first, true)

	if !first.set || !first.enabled {
		t.Errorf("first store: set=%v enabled=%v, want set=true enabled=true", first.set, first.enabled)
	}
	if second.set || second.enabled {
		t.Errorf("second, unrelated store was touched by activating the first: set=%v enabled=%v, want both false",
			second.set, second.enabled)
	}
}

// TestActivateStoreReadsConfigAtConstructionTime covers done-when #1: a store
// whose versioned-history.enabled reads "true" gets SetVersionedHistoryEnabled(true)
// at construction time.
func TestActivateStoreReadsConfigAtConstructionTime(t *testing.T) {
	store := newVersionableStore("true")
	got, err := ActivateStore(context.Background(), store, nil)
	if err != nil {
		t.Fatalf("ActivateStore returned an error: %v", err)
	}
	if got != store {
		t.Fatal("ActivateStore must return the same store it was given on success")
	}
	if !store.set || !store.enabled {
		t.Errorf("set=%v enabled=%v, want both true for config value %q", store.set, store.enabled, "true")
	}
}

// TestActivateStoreDefaultsToDisabled pins the safe default for an operator
// who has never touched the key: off, matching the atomic.Bool's own zero
// value in every implementation, so a store that predates this bead behaves
// identically to one that has never had the key set.
func TestActivateStoreDefaultsToDisabled(t *testing.T) {
	store := newVersionableStore("") // key never set
	if _, err := ActivateStore(context.Background(), store, nil); err != nil {
		t.Fatalf("ActivateStore returned an error: %v", err)
	}
	if store.enabled {
		t.Error("an unset versioned-history.enabled must default to disabled")
	}
}

// TestActivateStoreFlippingBackToFalseNeedsNoRestart is done-when #3,
// exercised directly: two ActivateStore calls against the same store, with
// the config value changed in between exactly as a plain `bd config set`
// would change it — no rebuild, no restart, nothing resembling reconstructing
// the store in between.
func TestActivateStoreFlippingBackToFalseNeedsNoRestart(t *testing.T) {
	store := newVersionableStore("true")
	if _, err := ActivateStore(context.Background(), store, nil); err != nil {
		t.Fatalf("first ActivateStore returned an error: %v", err)
	}
	if !store.enabled {
		t.Fatal("expected enabled after the first activation")
	}

	store.config[ConfigKey] = "false" // the plain config change
	if _, err := ActivateStore(context.Background(), store, nil); err != nil {
		t.Fatalf("second ActivateStore returned an error: %v", err)
	}
	if store.enabled {
		t.Error("flipping the config back to false and re-activating must disable it — no rebuild or restart involved")
	}
}

// TestActivateStorePassesThroughAFailedOpen matches ActivateStore's sibling
// in internal/eventsjournal: a failed open must not be masked or acted on.
func TestActivateStorePassesThroughAFailedOpen(t *testing.T) {
	sentinel := errOpenFailed
	if _, err := ActivateStore(context.Background(), nil, sentinel); err != sentinel {
		t.Errorf("ActivateStore rewrote a failed open: %v", err)
	}
}

var errOpenFailed = &openError{}

type openError struct{}

func (*openError) Error() string { return "open failed" }

// compile-time proof the doubles above stand in for a real store.
var (
	_ storage.DoltStorage                = (*unversionableStore)(nil)
	_ storage.VersionedHistoryConfigurer = (*versionableStore)(nil)
)
