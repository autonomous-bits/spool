-- Adds the artifacts + chunk_artifacts tables (G08.SG2).
--
-- Authoritative source: Meridian IDEA-58/IDEA-59 (artifacts are strictly immutable blobs;
-- updating means uploading a brand-new artifact with a new id), IDEA-60 (chunks associate with
-- multiple artifacts, versioned per-branch under the same delta model as edges), IDEA-61/IDEA-85
-- (blob storage behind a swappable ArtifactBlobStore port; for this environment, local-filesystem
-- storage on a Docker-named-volume, not S3), IDEA-62 (chunk_artifacts junction table shape:
-- branch_id, origin_branch_id, status, audit columns), IDEA-64 (partial unique index preventing
-- duplicate active mainline associations).
--
-- `artifacts` has no updated_at/mutable column: per IDEA-59 no domain path may rewrite an
-- existing artifact's blob reference in place, so there is nothing to timestamp an update for.
CREATE TABLE artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    uri TEXT NOT NULL,
    mime_type VARCHAR(255) NOT NULL,
    created_by_stakeholder_id UUID NOT NULL REFERENCES stakeholders(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Mirrors the edges table's delta-model shape (IDEA-32/IDEA-62): chunk_label is a logical label,
-- not an FK, per the same branch-overlay precedent as edges.from_chunk_label/to_chunk_label
-- (label identity is scope-resolved at read time, not enforced at the DB level).
CREATE TABLE chunk_artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chunk_label VARCHAR(50) NOT NULL,
    artifact_id UUID NOT NULL REFERENCES artifacts(id),
    status VARCHAR(50) NOT NULL CHECK (status IN ('active', 'superseded', 'deactivated')),
    branch_id UUID REFERENCES branches(id) ON DELETE CASCADE,
    origin_branch_id UUID REFERENCES branches(id),
    created_by_stakeholder_id UUID NOT NULL REFERENCES stakeholders(id),
    updated_by_stakeholder_id UUID NOT NULL REFERENCES stakeholders(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chunk_artifacts_branch_lookup ON chunk_artifacts (branch_id, chunk_label, artifact_id);

-- IDEA-64: partial unique index preventing duplicate active mainline associations.
CREATE UNIQUE INDEX idx_chunk_artifacts_mainline ON chunk_artifacts (chunk_label, artifact_id) WHERE branch_id IS NULL AND status = 'active';
