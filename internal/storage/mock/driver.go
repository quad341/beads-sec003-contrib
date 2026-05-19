// Package mock provides an in-memory storage.Driver for testing.
// It satisfies CapCRUD + CapRowExport + CapRowImport but not CapVersionControl or CapSync.
// Use storage.RegisterDriver("mock", mock.New) to make it available via storage.OpenDriver.
package mock

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// errNotSupported is returned by Storage methods the mock driver does not implement.
var errNotSupported = errors.New("mock: operation not supported")

var mockCaps = storage.NewCapabilitySet(
	storage.CapCRUD,
	storage.CapRowExport,
	storage.CapRowImport,
)

// New constructs a MemDriver. Open must be called before use.
func New() (storage.Driver, error) {
	return &MemDriver{
		issues: make(map[string]*types.Issue),
		config: make(map[string]string),
		meta:   make(map[string]string),
	}, nil
}

// MemDriver is an in-memory implementation of storage.Driver used in tests.
type MemDriver struct {
	mu     sync.RWMutex
	issues map[string]*types.Issue
	config map[string]string
	meta   map[string]string
}

// --- storage.Driver methods ---

func (d *MemDriver) Name() string                                         { return "mock" }
func (d *MemDriver) Capabilities() storage.CapabilitySet                  { return mockCaps }
func (d *MemDriver) Open(_ context.Context, _ storage.DriverConfig) error { return nil }
func (d *MemDriver) Ping(_ context.Context) error                         { return nil }
func (d *MemDriver) SchemaVersion(_ context.Context) (int, error)         { return 1, nil }
func (d *MemDriver) InitSchema(_ context.Context) error                   { return nil }
func (d *MemDriver) MigrateSchema(_ context.Context, _ int) error         { return nil }
func (d *MemDriver) Close() error                                         { return nil }

// --- storage.Storage: Issue CRUD ---

func (d *MemDriver) CreateIssue(_ context.Context, issue *types.Issue, _ string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if issue == nil || issue.ID == "" {
		return fmt.Errorf("mock: issue or ID must not be empty")
	}
	if _, exists := d.issues[issue.ID]; exists {
		return fmt.Errorf("mock: issue %q already exists", issue.ID)
	}
	cp := *issue
	d.issues[issue.ID] = &cp
	return nil
}

func (d *MemDriver) CreateIssues(ctx context.Context, issues []*types.Issue, actor string) error {
	for _, iss := range issues {
		if err := d.CreateIssue(ctx, iss, actor); err != nil {
			return err
		}
	}
	return nil
}

func (d *MemDriver) GetIssue(_ context.Context, id string) (*types.Issue, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	iss, ok := d.issues[id]
	if !ok {
		return nil, fmt.Errorf("mock: issue %q not found", id)
	}
	cp := *iss
	return &cp, nil
}

func (d *MemDriver) GetIssueByExternalRef(_ context.Context, ref string) (*types.Issue, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, iss := range d.issues {
		if iss.ExternalRef != nil && *iss.ExternalRef == ref {
			cp := *iss
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("mock: no issue with external_ref %q", ref)
}

func (d *MemDriver) GetIssuesByIDs(ctx context.Context, ids []string) ([]*types.Issue, error) {
	var out []*types.Issue
	for _, id := range ids {
		iss, err := d.GetIssue(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, iss)
	}
	return out, nil
}

func (d *MemDriver) UpdateIssue(_ context.Context, id string, updates map[string]interface{}, _ string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	iss, ok := d.issues[id]
	if !ok {
		return fmt.Errorf("mock: issue %q not found", id)
	}
	if title, ok := updates["title"].(string); ok {
		iss.Title = title
	}
	if status, ok := updates["status"].(types.Status); ok {
		iss.Status = status
	} else if statusStr, ok := updates["status"].(string); ok {
		iss.Status = types.Status(statusStr)
	}
	return nil
}

func (d *MemDriver) ReopenIssue(_ context.Context, id, _, _ string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	iss, ok := d.issues[id]
	if !ok {
		return fmt.Errorf("mock: issue %q not found", id)
	}
	iss.Status = types.StatusOpen
	return nil
}

func (d *MemDriver) UpdateIssueType(_ context.Context, id, issueType, _ string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	iss, ok := d.issues[id]
	if !ok {
		return fmt.Errorf("mock: issue %q not found", id)
	}
	iss.IssueType = types.IssueType(issueType)
	return nil
}

func (d *MemDriver) CloseIssue(_ context.Context, id, reason, _, _ string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	iss, ok := d.issues[id]
	if !ok {
		return fmt.Errorf("mock: issue %q not found", id)
	}
	iss.Status = types.StatusClosed
	iss.CloseReason = reason
	return nil
}

func (d *MemDriver) DeleteIssue(_ context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.issues[id]; !ok {
		return fmt.Errorf("mock: issue %q not found", id)
	}
	delete(d.issues, id)
	return nil
}

func (d *MemDriver) SearchIssues(_ context.Context, query string, filter types.IssueFilter) ([]*types.Issue, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	var out []*types.Issue
	for _, iss := range d.issues {
		if query != "" && !strings.Contains(iss.Title, query) {
			continue
		}
		if filter.Status != nil && iss.Status != *filter.Status {
			continue
		}
		cp := *iss
		out = append(out, &cp)
	}
	return out, nil
}

// --- storage.Storage: unsupported operations return errNotSupported ---

func (d *MemDriver) AddDependency(_ context.Context, _ *types.Dependency, _ string) error {
	return errNotSupported
}
func (d *MemDriver) RemoveDependency(_ context.Context, _, _, _ string) error {
	return errNotSupported
}
func (d *MemDriver) GetDependencies(_ context.Context, _ string) ([]*types.Issue, error) {
	return nil, errNotSupported
}
func (d *MemDriver) GetDependents(_ context.Context, _ string) ([]*types.Issue, error) {
	return nil, errNotSupported
}
func (d *MemDriver) GetDependenciesWithMetadata(_ context.Context, _ string) ([]*types.IssueWithDependencyMetadata, error) {
	return nil, errNotSupported
}
func (d *MemDriver) GetDependentsWithMetadata(_ context.Context, _ string) ([]*types.IssueWithDependencyMetadata, error) {
	return nil, errNotSupported
}
func (d *MemDriver) GetDependencyTree(_ context.Context, _ string, _ int, _, _ bool) ([]*types.TreeNode, error) {
	return nil, errNotSupported
}
func (d *MemDriver) AddLabel(_ context.Context, _, _, _ string) error    { return errNotSupported }
func (d *MemDriver) RemoveLabel(_ context.Context, _, _, _ string) error { return errNotSupported }
func (d *MemDriver) GetLabels(_ context.Context, _ string) ([]string, error) {
	return nil, errNotSupported
}
func (d *MemDriver) GetIssuesByLabel(_ context.Context, _ string) ([]*types.Issue, error) {
	return nil, errNotSupported
}
func (d *MemDriver) GetReadyWork(_ context.Context, _ types.WorkFilter) ([]*types.Issue, error) {
	return nil, errNotSupported
}
func (d *MemDriver) GetBlockedIssues(_ context.Context, _ types.WorkFilter) ([]*types.BlockedIssue, error) {
	return nil, errNotSupported
}
func (d *MemDriver) GetEpicsEligibleForClosure(_ context.Context) ([]*types.EpicStatus, error) {
	return nil, errNotSupported
}
func (d *MemDriver) ListWisps(_ context.Context, _ types.WispFilter) ([]*types.Issue, error) {
	return nil, errNotSupported
}
func (d *MemDriver) AddIssueComment(_ context.Context, _, _, _ string) (*types.Comment, error) {
	return nil, errNotSupported
}
func (d *MemDriver) GetIssueComments(_ context.Context, _ string) ([]*types.Comment, error) {
	return nil, errNotSupported
}
func (d *MemDriver) GetEvents(_ context.Context, _ string, _ int) ([]*types.Event, error) {
	return nil, errNotSupported
}
func (d *MemDriver) GetAllEventsSince(_ context.Context, _ time.Time) ([]*types.Event, error) {
	return nil, errNotSupported
}

func (d *MemDriver) GetStatistics(_ context.Context) (*types.Statistics, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return &types.Statistics{TotalIssues: len(d.issues)}, nil
}

func (d *MemDriver) SetConfig(_ context.Context, key, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.config[key] = value
	return nil
}

func (d *MemDriver) GetConfig(_ context.Context, key string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.config[key], nil
}

func (d *MemDriver) GetAllConfig(_ context.Context) (map[string]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make(map[string]string, len(d.config))
	for k, v := range d.config {
		out[k] = v
	}
	return out, nil
}

func (d *MemDriver) SetLocalMetadata(_ context.Context, key, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.meta[key] = value
	return nil
}

func (d *MemDriver) GetLocalMetadata(_ context.Context, key string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.meta[key], nil
}

func (d *MemDriver) RunInTransaction(_ context.Context, _ string, fn func(storage.Transaction) error) error {
	return errNotSupported
}

func (d *MemDriver) MergeSlotCreate(_ context.Context, _ string) (*types.Issue, error) {
	return nil, errNotSupported
}

func (d *MemDriver) MergeSlotCheck(_ context.Context) (*storage.MergeSlotStatus, error) {
	return nil, errNotSupported
}

func (d *MemDriver) MergeSlotAcquire(_ context.Context, _, _ string, _ bool) (*storage.MergeSlotResult, error) {
	return nil, errNotSupported
}

func (d *MemDriver) MergeSlotRelease(_ context.Context, _, _ string) error { return errNotSupported }

func (d *MemDriver) SlotSet(_ context.Context, _, _, _, _ string) error { return errNotSupported }
func (d *MemDriver) SlotGet(_ context.Context, _, _ string) (string, error) {
	return "", errNotSupported
}
func (d *MemDriver) SlotClear(_ context.Context, _, _, _ string) error { return errNotSupported }
