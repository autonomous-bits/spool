-- Adds the pairing_codes table for the interactive OAuth loopback hand-off: the browser receives
-- only a short-lived opaque pairing code, while the store persists the minted session/refresh
-- tokens server-side until the CLI/MCP process exchanges that code exactly once.
--
-- SCOPE DEVIATION (none): schema only. Code hashing, consumption semantics, and default TTL policy
-- are implemented by later dependent todos.
CREATE TABLE pairing_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code_hash TEXT NOT NULL,
    session_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_pairing_codes_code_hash ON pairing_codes (code_hash);
