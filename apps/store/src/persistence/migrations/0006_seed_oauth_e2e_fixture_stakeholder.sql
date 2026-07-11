-- Seeds one additional stakeholder, distinct from the bootstrap stakeholder
-- (0002_seed_bootstrap_stakeholder.sql), that has both a non-null discipline and a github_login.
--
-- The bootstrap stakeholder deliberately has discipline IS NULL (stakeholder registration is out
-- of scope per G01/G02), so it cannot satisfy G04.SG4's discipline-match invariant on submit.
-- G04.SG5's Docker end-to-end exercise needs a known-valid stakeholder whose GitHub login maps
-- through the (stubbed) OAuth callback to a stakeholder with a real discipline, so branch
-- submission can be exercised over real HTTP. Fixed UUID and github_login are documented in
-- apps/store/src/persistence/oauth-e2e-fixture-stakeholder.ts; keep all three in sync.
--
-- Idempotent: safe to re-run against the same database.
INSERT INTO stakeholders (id, name, email, role, discipline, github_login)
VALUES (
    '00000000-0000-0000-0000-000000000002',
    'OAuth E2E Fixture Stakeholder',
    'oauth-e2e-fixture-stakeholder@spool.local',
    'stakeholder',
    'engineering',
    'spool-e2e-oauth-fixture'
)
ON CONFLICT (id) DO NOTHING;
