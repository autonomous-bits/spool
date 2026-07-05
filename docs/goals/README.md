# Spool goals index

Repo-local record of goals selected from Meridian and their execution plan. This index does not
restate Meridian's authority — see `docs/constitution.md` § Meridian authority.

| Goal | Slug | Summary | Status | Artifact | Meridian chunks |
|------|------|---------|--------|----------|------------------|
| G01 | atomic-chunk-capture | Foundational vertical slice: capture and retrieve an atomic idea chunk via NestJS API + Postgres + MCP tool. | done | [goal.html](./G01-atomic-chunk-capture/goal.html) | IDEA-1, IDEA-2, IDEA-12, IDEA-4, IDEA-14, IDEA-11, IDEA-50, IDEA-51, IDEA-52, IDEA-34, IDEA-31, IDEA-77 (promoted ADR amendment resolving schema-gap), IDEA-78 (promoted ADR amendment ratifying branchless/draft capture), IDEA-72, IDEA-73, IDEA-9, IDEA-76 (promoted gap-report + recommendation; both recommendations now resolved via IDEA-77/IDEA-78) |
| G02 | branch-creation-and-scoped-authoring | Branch creation + branch-scoped chunk authoring via NestJS API + Postgres + MCP tools, unlocking the merge/submission pipeline. | done | [goal.html](./G02-branch-creation-and-scoped-authoring/goal.html) | IDEA-24, IDEA-17, IDEA-40, IDEA-41, IDEA-74, IDEA-53, IDEA-9, IDEA-52, IDEA-34, IDEA-31, IDEA-32, IDEA-33, IDEA-77, IDEA-78 |
| G03 | typed-edge-creation | Typed edge creation between chunks (branch-scoped, label-referenced) via NestJS API + Postgres + MCP tool, the other core pillar of the graph domain model alongside chunks. | done | [goal.html](./G03-typed-edge-creation/goal.html) | IDEA-19, IDEA-26, IDEA-36, IDEA-37, IDEA-38, IDEA-9, IDEA-52, IDEA-34, IDEA-31, IDEA-32, IDEA-33, IDEA-44 |
| G04 | branch-submission | Branch submission (draft → submitted) via NestJS API + Postgres, gated by GitHub OAuth-derived human session tokens (IDEA-81), enforcing discipline-matched atomic transition and hardening chunk/edge writes against it; first phase of the Merge Pipeline. | done | [goal.html](./G04-branch-submission/goal.html) | IDEA-18, IDEA-20, IDEA-17, IDEA-35, IDEA-40, IDEA-53, IDEA-75, IDEA-57, IDEA-9, IDEA-52, IDEA-34, IDEA-31, IDEA-79, IDEA-81 |
