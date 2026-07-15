-- Seed exactly one bootstrap stakeholder so chunk capture has a valid
-- created_by_stakeholder_id / updated_by_stakeholder_id to reference before real stakeholder
-- registration exists (out of scope for G01 per the goal's open questions).
--
-- Fixed UUID is documented in apps/store/src/persistence/bootstrap-stakeholder.ts; keep both in
-- sync. Idempotent: safe to re-run against the same database.
--
-- Always seeded with the placeholder name/email (not ADMIN_STAKEHOLDER_NAME/EMAIL) because later
-- migrations (0013) add a FK from workspaces.created_by_stakeholder_id to this row, so it must
-- exist by migration order, before migrator.ts's ensureBaselineSeedData (which runs after all
-- migration files) has a chance to run. ensureBaselineSeedData subsequently updates this row's
-- name/email in place from ADMIN_STAKEHOLDER_NAME/EMAIL when set (see there for details).
INSERT INTO stakeholders (id, name, email, role, discipline)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Bootstrap Stakeholder',
    'bootstrap-stakeholder@spool.local',
    'system',
    NULL
)
ON CONFLICT (id) DO NOTHING;
