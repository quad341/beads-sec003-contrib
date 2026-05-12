-- 0004_rename_friendly_fks.down.sql — be-lj68rq revert
--
-- Restore the original ON DELETE CASCADE FKs (no ON UPDATE CASCADE).
-- Reverting this loses the rename-without-FK-dance capability; callers of
-- UpdateIssueID will then fail until the up migration is reapplied.

ALTER TABLE dependencies         DROP CONSTRAINT IF EXISTS fk_dep_issue;
ALTER TABLE dependencies         ADD  CONSTRAINT fk_dep_issue       FOREIGN KEY (issue_id)
    REFERENCES issues(id) ON DELETE CASCADE;

ALTER TABLE labels               DROP CONSTRAINT IF EXISTS fk_labels_issue;
ALTER TABLE labels               ADD  CONSTRAINT fk_labels_issue    FOREIGN KEY (issue_id)
    REFERENCES issues(id) ON DELETE CASCADE;

ALTER TABLE comments             DROP CONSTRAINT IF EXISTS fk_comments_issue;
ALTER TABLE comments             ADD  CONSTRAINT fk_comments_issue  FOREIGN KEY (issue_id)
    REFERENCES issues(id) ON DELETE CASCADE;

ALTER TABLE events               DROP CONSTRAINT IF EXISTS fk_events_issue;
ALTER TABLE events               ADD  CONSTRAINT fk_events_issue    FOREIGN KEY (issue_id)
    REFERENCES issues(id) ON DELETE CASCADE;

ALTER TABLE issue_snapshots      DROP CONSTRAINT IF EXISTS fk_snapshots_issue;
ALTER TABLE issue_snapshots      ADD  CONSTRAINT fk_snapshots_issue FOREIGN KEY (issue_id)
    REFERENCES issues(id) ON DELETE CASCADE;

ALTER TABLE compaction_snapshots DROP CONSTRAINT IF EXISTS fk_comp_snap_issue;
ALTER TABLE compaction_snapshots ADD  CONSTRAINT fk_comp_snap_issue FOREIGN KEY (issue_id)
    REFERENCES issues(id) ON DELETE CASCADE;
