-- Adds the stakeholder_disciplines join table for G21 (Multi-Discipline Stakeholders), per
-- Meridian IDEA-142 (product: single active discipline, chosen per-request, scoped per-workspace,
-- validated server-side) and IDEA-143 (architecture: a new per-workspace allow-list join table,
-- WorkspaceMembership stays flat/unextended).
--
-- Composite FK (workspace_id, stakeholder_id) -> workspace_memberships(workspace_id,
-- stakeholder_id) ON DELETE CASCADE: prevents assigning an allowed discipline to a
-- (workspace, stakeholder) pair that isn't already a membership, and automatically drops stale
-- allow-list rows when a membership is removed.
--
-- stakeholders.discipline (the legacy single-column source, IDEA-141 constraint #1) is NOT
-- dropped or modified by this migration; it remains the backfill source only.
CREATE TABLE stakeholder_disciplines (
    workspace_id UUID NOT NULL,
    stakeholder_id UUID NOT NULL,
    discipline VARCHAR(50) NOT NULL CHECK (discipline IN ('product', 'architecture', 'design', 'engineering', 'security', 'governance')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, stakeholder_id, discipline),
    FOREIGN KEY (workspace_id, stakeholder_id)
        REFERENCES workspace_memberships (workspace_id, stakeholder_id)
        ON DELETE CASCADE
);

-- Backfill: every existing workspace_membership whose stakeholder has a non-null legacy
-- discipline keeps that discipline as their sole allowed discipline in every workspace they
-- already belong to.
INSERT INTO stakeholder_disciplines (workspace_id, stakeholder_id, discipline)
SELECT wm.workspace_id, wm.stakeholder_id, s.discipline
  FROM workspace_memberships wm
  JOIN stakeholders s ON s.id = wm.stakeholder_id
 WHERE s.discipline IS NOT NULL
ON CONFLICT DO NOTHING;
