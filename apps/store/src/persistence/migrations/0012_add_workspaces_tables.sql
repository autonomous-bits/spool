-- Adds the authoritative workspaces and workspace_memberships tables for G10 (Workspace Registry
-- & Membership Bootstrap), matching Meridian IDEA-96's ratified ADR column-for-column.
--
-- Authoritative source: Meridian IDEA-96 (promoted ADR: workspaces/workspace_memberships schema,
-- no role column), IDEA-95 (promoted: flat no-roles membership, creator is simply the first
-- membership row), IDEA-88/IDEA-89 (promoted: workspace as a project/product-line scope).
--
-- SCOPE DEVIATION (none): this migration deliberately does NOT add a workspace_id column, index,
-- or constraint to any existing table (chunks, edges, branches, artifacts, chunk_artifacts,
-- suggestions, verification_signals, feedback_notifications). Meridian IDEA-90 (workspace
-- isolation / label scoping of existing tables) is promoted but explicitly out of scope for this
-- goal — see docs/goals/G10-workspace-registry-membership OQ1.
CREATE TABLE workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    created_by_stakeholder_id UUID NOT NULL REFERENCES stakeholders(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE workspace_memberships (
    workspace_id UUID NOT NULL REFERENCES workspaces(id),
    stakeholder_id UUID NOT NULL REFERENCES stakeholders(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, stakeholder_id)
);
