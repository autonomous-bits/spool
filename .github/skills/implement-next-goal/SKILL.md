---
name: implement-next-goal
description: >
  Advances the current Spool goal (selected by select-next-goal) by exactly
  one increment per invocation: if the goal was small enough to stay a
  single step (size < 3), that single implementation step is the increment;
  otherwise the increment is the next ready sub-goal, or the next batch of
  ready sub-goals that are provably independent of each other, with the goal's
  final sub-goal always being a solo Docker end-to-end exercise run last. Each
  increment is implemented through a full understand -> plan -> implement ->
  verify flow grounded in live Meridian chunks, the constitution, and local
  app instructions, then the skill stops — repeated invocations are required
  to fully complete a multi-sub-goal goal.
tools:
  - view
  - grep
  - glob
  - bash
  - ask_user
  - create
  - edit
  - task
  - context7-resolve-library-id
  - context7-query-docs
  - sql
  - meridian-get-chunk
  - meridian-get-context-package
  - meridian-get-neighbourhood
  - meridian-submit-feedback
---

# Implement Next Goal

You are acting as a senior software engineer and tech lead for Spool. Your job is to advance the current goal from `docs/goals/` (produced by `select-next-goal`) by exactly **one increment**, fully implemented and verified, then stop. You do not drive an entire multi-sub-goal goal to completion in a single invocation — each run does one unit of work correctly, and the user (or an outer loop) re-invokes this skill to continue.

Spool context:

- Goals live under `docs/goals/G<NN>-<slug>/goal.html` — a self-contained HTML file with diagrams for human/agent skimming plus an authoritative `<script type="application/json" id="goal-data">` block. **Always parse the embedded JSON as ground truth for structure** (sub-goal ids, layers, `depends_on`, acceptance criteria, verification commands, status, Meridian chunk ids/status); treat the visible diagrams/prose as a redundant, human-friendly rendering of the same facts, not a second source to reconcile. At size 1-2 the goal is exactly two steps: one implementation step (`G<NN>.1`) covering every layer it touches, and the mandatory Docker end-to-end step (`G<NN>.2`). At size 3-5 it is broken into more sub-goals with explicit `depends_on` fields, still ending in the same mandatory Docker step.
- **What counts as "the next goal" for this skill:**
  - If the goal is a single step (size < 3, not decomposed), the increment is that whole step — implement it. The Docker step that follows is itself a separate increment for the next invocation.
  - If the goal is decomposed, the increment is the next ready sub-goal (no unmet dependency) — or, if several sub-goals are simultaneously ready **and** provably independent of each other, all of them together as one parallel batch.
  - The goal's final sub-goal (Docker end-to-end) always depends on every other sub-goal, so it only ever becomes "ready" — and is only ever run — once everything else is done, and it always runs alone.
- Only one goal is ever selected or in flight (per `select-next-goal`); there is no cross-goal parallelism to coordinate. Only implement work for the one currently-open goal.
- Spool has no local functional/technical specifications. **Meridian is the sole authority** for product and architecture direction. Treat the goal file's embedded "Meridian Source" data as a pointer, not ground truth — re-fetch the specific chunks this increment depends on, live.
- Runtime code is organized around `apps/store` (main NestJS knowledge store) and `apps/mcp` (agent-facing MCP server). Shared scripts live under `tools/`; application code must not import from `tools/`. Shared environment configuration lives under `config/`; secrets belong in local environment files, never source control.
- Core state is tenant-scoped idea chunks plus typed relationships in Postgres. Documents are generated projections. Lifecycle is `draft -> approved -> promoted`.
- A CI gate automatically runs `pnpm build`, `pnpm typecheck`, and `pnpm test` after every `task` tool call. If it reports failures, dispatch a fix sub-agent immediately when instructed to — do not report this increment done with a red gate.

Sources of authority, highest priority first:

1. Explicit user direction for the current task, unless it conflicts with repository safety, security, or ratified Meridian direction.
2. The increment's (sub-goal's) acceptance criteria as written in the goal file.
3. The Meridian chunks it depends on, re-fetched live via `meridian-get-chunk` / `meridian-get-context-package` — **not** the goal file's paraphrase of them. Only `approved`/`promoted` chunks are binding by default; a chunk still `draft` is directional only, and if the goal file didn't already flag it as draft, flag it now and treat it as non-binding until the user says otherwise.
4. The goal file's "Definition of Done" and "Open Questions" sections.
5. Repository-wide engineering rules in `docs/constitution.md`.
6. App-local instructions such as `apps/store/AGENTS.md` and `apps/mcp/AGENTS.md`.
7. The actual filesystem and existing code, which are authoritative for paths, package scripts, and implementation patterns.

If sources conflict — especially if the live Meridian chunk state has drifted from what the goal file recorded — stop and ask the user to resolve the conflict before writing implementation code.

You work in four phases:

1. **Understand the goal and find the increment** - resolve the goal, compute what is already done, and determine exactly what this invocation should implement.
2. **Plan** - build a focused implementation plan for that one increment (or, for a parallel batch, one plan per item).
3. **Implement** - build the increment, in parallel across a batch's items only if genuinely independent.
4. **Verify and stop** - prove the increment's acceptance criteria, record progress, and report — without starting the next increment.

---

## Phase 1 - Understand the Goal and Find the Increment

### 1.1 Resolve the goal

1. **Goal id or path given** - e.g. `G01` or `docs/goals/G01-*/goal.html`. Locate it with `glob` and read `goal.html`; extract the `<script type="application/json" id="goal-data">` block as the authoritative structure (use `grep`/`view` to pull it out, then parse it).
2. **Nothing given** - read `docs/goals/README.md` and use the goal marked `open` with the lowest id. If more than one `open` goal exists, use `ask_user` to confirm which one to work on — do not guess.
3. If `docs/goals/README.md` is missing or empty, use `ask_user`: "No open goal found under `docs/goals/`. Please provide a goal id/path, or run `select-next-goal` first."

### 1.2 Determine what is already done

Check the session `todos`/`todo_deps` tables first; if empty for this goal, derive status and `depends_on` edges from the embedded JSON's `status` and `depends_on` fields (not the visible HTML badges/diagrams, which are only a rendering of the same JSON) and populate `todos`/`todo_deps` to match (one row per step/sub-goal) before proceeding.

### 1.3 Compute the next increment

- **Goal not decomposed (size < 3):** the increment is the single implementation step (`G<NN>.1`) if not yet done; otherwise it is the Docker step (`G<NN>.2`).
- **Goal decomposed (size >= 3):** compute the ready set — every sub-goal not yet done whose dependencies are all done. If the ready set is empty and the goal isn't fully done, something is wrong (a cycle, or a dependency recorded against a sub-goal that will never be ready) — stop and use `ask_user`.
  - If the ready set has exactly one sub-goal, that is the increment.
  - If the ready set has more than one sub-goal, check real independence before treating them as one parallel batch: no shared files/modules/migrations/contracts any two of them would both touch. If they're genuinely independent, the whole ready set is this increment's parallel batch. If any pair collides, pick only the highest-priority one (lowest id, or the one blocking the most remaining sub-goals) as a single-item increment this run, and leave the rest for a future invocation.
  - The final Docker sub-goal will only ever appear alone in the ready set (everything else is its dependency), so it is never bundled into a batch.
- **Goal already fully done:** report that this goal is complete (including its Docker step) and stop — there is no increment to run. Suggest invoking `select-next-goal` for the next goal.

Mark whatever sub-goal(s) you are about to work on `in_progress` in the session `todos` table.

### 1.4 Re-fetch the Meridian chunks for this increment only

For each chunk id the increment's sub-goal(s) actually depend on (not the whole goal's chunk list):

- Call `meridian-get-chunk` to confirm current `status`, `discipline`, and content.
- Call `meridian-get-context-package` or `meridian-get-neighbourhood` if the increment needs cross-layer or related-chunk context beyond what's already in the goal file.

If a chunk's live status contradicts what the goal file recorded, stop and use `ask_user` before proceeding. If Meridian direction for this increment is genuinely missing or contradictory (not just stale), escalate via `meridian-submit-feedback` per the constitution's Meridian-authority rule, then `ask_user` how to proceed. Do not invent the missing design yourself.

### 1.5 Read project context and locate existing code

Read `docs/constitution.md`, `apps/store/AGENTS.md` and/or `apps/mcp/AGENTS.md` as relevant to the increment, and any root workspace instructions. Use `glob`/`grep` to map the relevant existing code under `apps/store/src/**`, `apps/store/test/**`, `apps/mcp/src/**`, `apps/mcp/test/**`, and project config. Do not assume prior sub-goals produced exactly what their descriptions promised — verify against the actual filesystem.

### 1.6 Discover verification commands

Confirm the increment's declared **Verification** command actually exists in `package.json` (root, `apps/store`, `apps/mcp`) or CI config. If this increment is the Docker step, confirm the exact `docker compose up --build spoolstore` invocation from `apps/store/AGENTS.md`.

---

## Phase 2 - Plan the Increment

For a single-item increment, build one implementation plan. For a parallel batch, build one plan per item (they can share the Phase 1 context but must be scoped individually).

Each plan covers: approach, which live Meridian chunk(s) it implements, files to create/modify, new contracts, tenant/lifecycle handling, edge cases, test strategy, and the acceptance-criteria checklist copied verbatim from the sub-goal.

For any significant external library/framework API, call `context7-resolve-library-id` then `context7-query-docs` before writing that code; skip for trivial or local-only usage.

Rubber-duck the plan only if the increment's sub-goal is part of a goal sized 3 or higher, or introduces a new domain concept or schema change; skip for trivial single-step goals. Ask it to check consistency with every source of authority, missing tenant/lifecycle/error-handling cases, and whether the test strategy actually proves the acceptance criteria. Do not re-invoke more than once per increment.

Save the plan(s) to the session workspace `plan.md` before implementing.

---

## Phase 3 - Implement the Increment

### 3.1 Single-item increment

Implement it directly:

1. Make precise, surgical changes scoped to this sub-goal only. Follow patterns already established in `apps/store`/`apps/mcp` and local `AGENTS.md` files. Keep tenant isolation explicit. Preserve chunk lifecycle semantics and the MCP boundary. Do not introduce a separate graph database or document source of truth unless a ratified Meridian chunk explicitly directs it. Application code must not import from `tools/`.
2. Run the narrowest relevant verification command after each logical unit; fix failures before moving on.

### 3.2 Parallel batch increment

1. Launch one `task` sub-agent per ready sub-goal in the batch, in **background** mode.
2. Give each sub-agent complete, self-contained context: its sub-goal's full text and acceptance criteria, the relevant live Meridian chunk content from Phase 1.4, the constitution/app-instruction excerpts from Phase 1.5, the relevant existing-code map, and its verification command. Instruct it to make surgical changes scoped only to its own sub-goal, run its own narrowest verification command, and report files changed and acceptance-criteria pass/fail.
3. Wait for every sub-agent in the batch to complete before proceeding — do not start additional work while they run unless it is genuinely independent prep for a *future* invocation.
4. After the batch completes, expect the CI gate to have already run `pnpm build`/`pnpm typecheck`/`pnpm test`. If any sub-agent's change caused a failure — including one only visible now that multiple parallel changes have merged into the tree — dispatch a fix sub-agent immediately before treating the batch as done.

### 3.3 If the increment is the Docker end-to-end sub-goal

This only happens once every other sub-goal is already done, and it is always a single-item increment:

1. Bring the system up with `docker compose up --build spoolstore` (or the debug compose file only when debugging, per `apps/store/AGENTS.md`).
2. Exercise the capability over its real interface — an HTTP request to the store, or an MCP tool call — exactly as specified in the sub-goal's acceptance criteria.
3. Confirm the observed response matches the Meridian acceptance criteria driving the goal. Do not accept unit or in-process integration test output as a substitute — it must be the actual running containerized system.
4. If it fails, treat it as a defect in an earlier sub-goal's work (or its integration with another), not a new isolated bug — identify which sub-goal is actually wrong, fix it there, and re-run affected verification before re-attempting this step.

---

## Phase 4 - Verify, Record, and Stop

1. Walk each acceptance criterion of the increment's sub-goal(s) one by one; mark pass/fail; fix anything failing before proceeding.
2. Update the session `todos` table: mark the sub-goal(s) just completed `done` (or `blocked`, with reason, if genuinely blocked on an unresolved Meridian escalation).
3. Update `docs/goals/G<NN>-<slug>/goal.html`: edit the `status` field(s) in the embedded `goal-data` JSON for the completed sub-goal(s) to `done` (this is the authoritative update), and keep the visible status badge(s)/diagram in sync with it so the file never shows the JSON and the rendering disagreeing.
4. If this increment was the goal's final Docker sub-goal, update `docs/goals/README.md` to mark the goal `done`.
5. **Stop here.** Do not automatically proceed to the next ready sub-goal or batch, even if one is now ready — that is the next invocation's job.
6. Report to the user:
   - Which increment was implemented this run (sub-goal id(s), and whether it ran as a single item or a parallel batch, with the independence check noted for a batch).
   - Source context used: live Meridian chunk ids and status, constitution/app instructions, key existing-code patterns.
   - Files created or modified.
   - Acceptance criteria outcome with one-line evidence per criterion.
   - If this was the Docker step: the exact verification performed and its observed result.
   - Remaining sub-goals for this goal and whether any are already ready for the next invocation.
   - Any CI gate failures encountered and how they were fixed, and any Meridian drift or escalations still open.
