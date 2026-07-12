-- Adds the refresh_tokens table for the interactive OAuth refresh grant, per the MCP interactive
-- GitHub login plan's store-side schema requirements: each issued refresh token is persisted as a
-- revocable, rotatable row keyed by a hash of the opaque token value (never the raw token).
--
-- SCOPE DEVIATION (none): schema only. Repository/service wiring, token hashing, rotation, and
-- expiry enforcement are implemented by later dependent todos.
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stakeholder_id UUID NOT NULL REFERENCES stakeholders(id),
    workspace_id UUID REFERENCES workspaces(id),
    token_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    replaced_by_id UUID NULL REFERENCES refresh_tokens(id)
);

CREATE INDEX idx_refresh_tokens_token_hash ON refresh_tokens (token_hash);
CREATE INDEX idx_refresh_tokens_stakeholder ON refresh_tokens (stakeholder_id);
