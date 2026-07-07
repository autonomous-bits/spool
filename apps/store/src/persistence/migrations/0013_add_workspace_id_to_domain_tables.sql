-- Adds workspace_id to the 8 pre-existing domain tables and re-keys their global uniqueness
-- constraints to be per-workspace (G11 SG1), per Meridian IDEA-90 (workspace isolation & label
-- scoping: chunk labels and other uniqueness become unique per workspace, not globally) and
-- IDEA-91 (workspace backfill: all pre-existing data is assigned to a single implicit default
-- workspace so nothing breaks after the column is introduced).
--
-- Schema-only migration: no repository query filtering changes yet (that is G11 SG3/SG4/SG5).
--
-- SCOPE DEVIATION (none; interim, closed by a later sub-goal): workspace_id is added
-- `NOT NULL DEFAULT <default-workspace-id>` rather than nullable-then-backfilled-then-NOT-NULL.
-- No repository yet supplies workspace_id on INSERT (that wiring is G11 SG4/SG5), so a column
-- default is what keeps every existing INSERT statement — and therefore every existing test —
-- working unchanged; every pre-existing AND newly-inserted-before-SG4/5 row lands in the same
-- default workspace until each table's repository starts passing an explicit workspace_id. A
-- future sub-goal should drop this DEFAULT once every write path supplies workspace_id
-- explicitly, so a caller can never silently omit it.

-- The default workspace itself has no special runtime behavior beyond being pre-migration data's
-- home (IDEA-91). Owned by the existing bootstrap stakeholder (migration 0002) so the NOT NULL
-- created_by_stakeholder_id FK on workspaces is satisfied without inventing a new identity.
INSERT INTO workspaces (id, name, created_by_stakeholder_id)
VALUES (
    '00000000-0000-0000-0000-00000000d0fa',
    'Default Workspace',
    '00000000-0000-0000-0000-000000000001'
)
ON CONFLICT (id) DO NOTHING;

-- Every pre-existing stakeholder becomes a member of the default workspace, so existing
-- delegated/session-token flows (which will start requiring X-Workspace-Id membership checks in
-- later sub-goals) keep working against pre-migration data without a separate bootstrap step.
INSERT INTO workspace_memberships (workspace_id, stakeholder_id)
SELECT '00000000-0000-0000-0000-00000000d0fa', s.id
FROM stakeholders s
ON CONFLICT DO NOTHING;

-- chunks --------------------------------------------------------------------------------------
ALTER TABLE chunks ADD COLUMN workspace_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-00000000d0fa';
ALTER TABLE chunks ADD CONSTRAINT chunks_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id);

DROP INDEX idx_chunks_mainline;
DROP INDEX idx_chunks_draft_mainline;
ALTER TABLE chunks DROP CONSTRAINT unique_label_per_branch;

ALTER TABLE chunks ADD CONSTRAINT unique_label_per_branch UNIQUE (workspace_id, label, branch_id);
CREATE UNIQUE INDEX idx_chunks_mainline ON chunks (workspace_id, label) WHERE branch_id IS NULL AND status = 'promoted';
CREATE UNIQUE INDEX idx_chunks_draft_mainline ON chunks (workspace_id, label) WHERE branch_id IS NULL AND status = 'draft';

DROP INDEX idx_chunks_branch_lookup;
CREATE INDEX idx_chunks_branch_lookup ON chunks (workspace_id, branch_id, label);

-- edges ---------------------------------------------------------------------------------------
ALTER TABLE edges ADD COLUMN workspace_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-00000000d0fa';
ALTER TABLE edges ADD CONSTRAINT edges_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id);

DROP INDEX idx_edges_mainline;
DROP INDEX idx_edges_branch_active;
CREATE UNIQUE INDEX idx_edges_mainline ON edges (workspace_id, from_chunk_label, to_chunk_label, type) WHERE branch_id IS NULL AND status = 'active';
CREATE UNIQUE INDEX idx_edges_branch_active ON edges (workspace_id, from_chunk_label, to_chunk_label, type, branch_id) WHERE status = 'active';

DROP INDEX idx_edges_branch_lookup;
CREATE INDEX idx_edges_branch_lookup ON edges (workspace_id, branch_id, from_chunk_label, to_chunk_label);

-- branches --------------------------------------------------------------------------------------
ALTER TABLE branches ADD COLUMN workspace_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-00000000d0fa';
ALTER TABLE branches ADD CONSTRAINT branches_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id);

DROP INDEX idx_active_branch_name;
CREATE UNIQUE INDEX idx_active_branch_name ON branches (workspace_id, name) WHERE status != 'merged';

-- artifacts -------------------------------------------------------------------------------------
ALTER TABLE artifacts ADD COLUMN workspace_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-00000000d0fa';
ALTER TABLE artifacts ADD CONSTRAINT artifacts_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id);

-- chunk_artifacts ---------------------------------------------------------------------------------
ALTER TABLE chunk_artifacts ADD COLUMN workspace_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-00000000d0fa';
ALTER TABLE chunk_artifacts ADD CONSTRAINT chunk_artifacts_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id);

DROP INDEX idx_chunk_artifacts_mainline;
CREATE UNIQUE INDEX idx_chunk_artifacts_mainline ON chunk_artifacts (workspace_id, chunk_label, artifact_id) WHERE branch_id IS NULL AND status = 'active';

DROP INDEX idx_chunk_artifacts_branch_lookup;
CREATE INDEX idx_chunk_artifacts_branch_lookup ON chunk_artifacts (workspace_id, branch_id, chunk_label, artifact_id);

-- suggestions -------------------------------------------------------------------------------------
ALTER TABLE suggestions ADD COLUMN workspace_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-00000000d0fa';
ALTER TABLE suggestions ADD CONSTRAINT suggestions_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id);

DROP INDEX idx_suggestions_unique;
CREATE UNIQUE INDEX idx_suggestions_unique ON suggestions (workspace_id, label, content);

-- verification_signals -----------------------------------------------------------------------------
ALTER TABLE verification_signals ADD COLUMN workspace_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-00000000d0fa';
ALTER TABLE verification_signals ADD CONSTRAINT verification_signals_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id);

-- feedback_notifications --------------------------------------------------------------------------
ALTER TABLE feedback_notifications ADD COLUMN workspace_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-00000000d0fa';
ALTER TABLE feedback_notifications ADD CONSTRAINT feedback_notifications_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id);

DROP INDEX idx_feedback_notifications_stakeholder;
CREATE INDEX idx_feedback_notifications_stakeholder ON feedback_notifications (workspace_id, stakeholder_id, status);
