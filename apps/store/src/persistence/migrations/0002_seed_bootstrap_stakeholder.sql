-- Seed exactly one bootstrap stakeholder so chunk capture has a valid
-- created_by_stakeholder_id / updated_by_stakeholder_id to reference before real stakeholder
-- registration exists (out of scope for G01 per the goal's open questions).
--
-- Fixed UUID is documented in apps/store/src/persistence/bootstrap-stakeholder.ts; keep both in
-- sync. Idempotent: safe to re-run against the same database.
INSERT INTO stakeholders (id, name, email, role, discipline)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Bootstrap Stakeholder',
    'bootstrap-stakeholder@spool.local',
    'system',
    NULL
)
ON CONFLICT (id) DO NOTHING;
