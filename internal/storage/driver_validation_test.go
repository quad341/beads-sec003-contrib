package storage_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/mock"
	"github.com/steveyegge/beads/internal/types"
)

// TestMain registers the mock driver before any tests in this package run.
func TestMain(m *testing.M) {
	storage.RegisterDriver("mock", mock.New)
	os.Exit(m.Run())
}

// TestMockDriverCanOpen verifies that storage.OpenDriver("mock", cfg) succeeds
// and returns a driver whose Name() is "mock".
func TestMockDriverCanOpen(t *testing.T) {
	ctx := context.Background()
	cfg := storage.DriverConfig{BeadsDir: t.TempDir()}

	d, err := storage.OpenDriver(ctx, "mock", cfg)
	if err != nil {
		t.Fatalf("OpenDriver(%q) returned error: %v", "mock", err)
	}
	if d == nil {
		t.Fatal("OpenDriver returned nil driver")
	}
	if d.Name() != "mock" {
		t.Errorf("driver.Name() = %q, want %q", d.Name(), "mock")
	}

	// Verify it implements storage.Driver.
	var _ storage.Driver = d
}

// TestCapabilitySetRequire_has verifies that Require on a present capability returns nil.
func TestCapabilitySetRequire_has(t *testing.T) {
	ctx := context.Background()
	d, err := storage.OpenDriver(ctx, "mock", storage.DriverConfig{})
	if err != nil {
		t.Fatalf("OpenDriver error: %v", err)
	}
	caps := d.Capabilities()

	for _, cap := range []storage.Capability{
		storage.CapCRUD,
		storage.CapRowExport,
		storage.CapRowImport,
	} {
		if err := caps.Require(cap, "mock"); err != nil {
			t.Errorf("Require(%q) returned unexpected error: %v", cap, err)
		}
	}
}

// TestCapabilitySetRequire_missing verifies that Require on an absent capability
// returns ErrCapabilityNotSupported with the driver name and capability in the message.
func TestCapabilitySetRequire_missing(t *testing.T) {
	ctx := context.Background()
	d, err := storage.OpenDriver(ctx, "mock", storage.DriverConfig{})
	if err != nil {
		t.Fatalf("OpenDriver error: %v", err)
	}
	caps := d.Capabilities()

	for _, cap := range []storage.Capability{
		storage.CapSync,
		storage.CapVersionControl,
		storage.CapPush,
		storage.CapPull,
	} {
		err := caps.Require(cap, "mock")
		if err == nil {
			t.Errorf("Require(%q): expected ErrCapabilityNotSupported, got nil", cap)
			continue
		}
		if !errors.Is(err, storage.ErrCapabilityNotSupported) {
			t.Errorf("Require(%q): errors.Is(err, ErrCapabilityNotSupported) = false, got: %v", cap, err)
		}
		msg := err.Error()
		if !strings.Contains(msg, string(cap)) {
			t.Errorf("Require(%q) error message %q does not contain capability name", cap, msg)
		}
		if !strings.Contains(msg, "mock") {
			t.Errorf("Require(%q) error message %q does not contain driver name", cap, msg)
		}
	}
}

// TestDriverRefusal verifies that calling Require for a Dolt-specific capability
// (CapVersionControl) on the mock driver returns ErrCapabilityNotSupported —
// not a panic and not a generic "command not found" error.
func TestDriverRefusal(t *testing.T) {
	ctx := context.Background()
	d, err := storage.OpenDriver(ctx, "mock", storage.DriverConfig{})
	if err != nil {
		t.Fatalf("OpenDriver error: %v", err)
	}
	caps := d.Capabilities()

	err = caps.Require(storage.CapVersionControl, d.Name())
	if err == nil {
		t.Fatal("Require(CapVersionControl) on mock driver: expected error, got nil")
	}
	if !errors.Is(err, storage.ErrCapabilityNotSupported) {
		t.Errorf("Require(CapVersionControl): want ErrCapabilityNotSupported, got: %v", err)
	}
	// Confirm it's neither a panic nor a "command not found" style error.
	msg := err.Error()
	if strings.Contains(msg, "command not found") {
		t.Errorf("Require error should not say 'command not found': %q", msg)
	}
}

// TestThirdDriverNoCommandLayerChanges verifies that adding the mock driver
// requires no changes to files in cmd/bd/. This is a structural test:
// if this file compiles and TestMockDriverCanOpen passes, the driver was
// registered purely via storage.RegisterDriver in TestMain — no cmd/bd edits needed.
// The test also exercises basic CRUD to confirm the driver is functional.
func TestThirdDriverNoCommandLayerChanges(t *testing.T) {
	ctx := context.Background()
	d, err := storage.OpenDriver(ctx, "mock", storage.DriverConfig{})
	if err != nil {
		t.Fatalf("OpenDriver(%q) error: %v", "mock", err)
	}

	issue := &types.Issue{
		ID:     "mock-001",
		Title:  "third driver test issue",
		Status: types.StatusOpen,
	}
	if err := d.CreateIssue(ctx, issue, "test"); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	got, err := d.GetIssue(ctx, "mock-001")
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if got.Title != issue.Title {
		t.Errorf("GetIssue title: got %q, want %q", got.Title, issue.Title)
	}

	stats, err := d.GetStatistics(ctx)
	if err != nil {
		t.Fatalf("GetStatistics: %v", err)
	}
	if stats.TotalIssues != 1 {
		t.Errorf("GetStatistics.TotalIssues = %d, want 1", stats.TotalIssues)
	}
}
