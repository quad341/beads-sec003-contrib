-- 0004_rename_friendly_fks.up.sql — be-lj68rq prereq
--
-- PG enforces FKs immediately by default. The initial schema (migration 0001)
-- declared ON DELETE CASCADE only — no ON UPDATE CASCADE. UpdateIssueID
-- (bd rename) needs to mutate issues.id in place; without ON UPDATE CASCADE,
-- the dependent rows in labels/comments/events/dependencies/issue_snapshots/
-- compaction_snapshots would be orphaned at the moment the issues row was
-- renamed and the UPDATE would fail.
--
-- Dolt sidesteps this with SET FOREIGN_KEY_CHECKS=0 (issueops/bulk_ops.go:226).
-- PG has no equivalent escape hatch that is both portable and safe, so we
-- harden the FKs once at migration time and let CASCADE do the work.
--
-- Bare ADD CONSTRAINT (not ADD CONSTRAINT NOT VALID) is intentional: PG
-- backend is LOCAL-ONLY in the v1 ship gate (be-nwwe75), and the prior
-- ON DELETE CASCADE FK already prevented orphans. If this migration fails on
-- a live DB, the schema was already corrupt and bd rename was unsafe anyway.

ALTER TABLE dependencies         DROP CONSTRAINT IF EXISTS fk_dep_issue;
ALTER TABLE dependencies         ADD  CONSTRAINT fk_dep_issue       FOREIGN KEY (issue_id)
    REFERENCES issues(id) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE labels               DROP CONSTRAINT IF EXISTS fk_labels_issue;
ALTER TABLE labels               ADD  CONSTRAINT fk_labels_issue    FOREIGN KEY (issue_id)
    REFERENCES issues(id) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE comments             DROP CONSTRAINT IF EXISTS fk_comments_issue;
ALTER TABLE comments             ADD  CONSTRAINT fk_comments_issue  FOREIGN KEY (issue_id)
    REFERENCES issues(id) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE events               DROP CONSTRAINT IF EXISTS fk_events_issue;
ALTER TABLE events               ADD  CONSTRAINT fk_events_issue    FOREIGN KEY (issue_id)
    REFERENCES issues(id) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE issue_snapshots      DROP CONSTRAINT IF EXISTS fk_snapshots_issue;
ALTER TABLE issue_snapshots      ADD  CONSTRAINT fk_snapshots_issue FOREIGN KEY (issue_id)
    REFERENCES issues(id) ON UPDATE CASCADE ON DELETE CASCADE;

ALTER TABLE compaction_snapshots DROP CONSTRAINT IF EXISTS fk_comp_snap_issue;
ALTER TABLE compaction_snapshots ADD  CONSTRAINT fk_comp_snap_issue FOREIGN KEY (issue_id)
    REFERENCES issues(id) ON UPDATE CASCADE ON DELETE CASCADE;
