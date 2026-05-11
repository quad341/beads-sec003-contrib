package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/steveyegge/beads/internal/storage/issueops"
	"github.com/steveyegge/beads/internal/types"
)

// syncCustomTypesPg replaces every row of custom_types with the parsed value.
// Mirrors the dolt-side issueops.SyncCustomTypesTable contract: the config row
// is the source of truth, the normalized table is a mirror. Empty value clears
// the table.
func syncCustomTypesPg(ctx context.Context, tx pgx.Tx, value string) error {
	if _, err := tx.Exec(ctx, "DELETE FROM custom_types"); err != nil {
		return fmt.Errorf("clear custom_types: %w", err)
	}
	if value == "" {
		return nil
	}
	for _, name := range issueops.ParseTypesValue(value) {
		if _, err := tx.Exec(ctx,
			"INSERT INTO custom_types (name) VALUES ($1)", name,
		); err != nil {
			return fmt.Errorf("insert custom_type %q: %w", name, err)
		}
	}
	return nil
}

// syncCustomStatusesPg replaces every row of custom_statuses with the parsed
// value. Mirrors issueops.SyncCustomStatusesTable.
func syncCustomStatusesPg(ctx context.Context, tx pgx.Tx, value string) error {
	if _, err := tx.Exec(ctx, "DELETE FROM custom_statuses"); err != nil {
		return fmt.Errorf("clear custom_statuses: %w", err)
	}
	if value == "" {
		return nil
	}
	parsed, err := types.ParseCustomStatusConfig(value)
	if err != nil {
		return fmt.Errorf("parse status.custom: %w", err)
	}
	for _, s := range parsed {
		if _, err := tx.Exec(ctx,
			"INSERT INTO custom_statuses (name, category) VALUES ($1, $2)",
			s.Name, string(s.Category),
		); err != nil {
			return fmt.Errorf("insert custom_status %q: %w", s.Name, err)
		}
	}
	return nil
}
