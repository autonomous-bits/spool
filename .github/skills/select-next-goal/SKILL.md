---
name: select-next-goal
description: >
  Decides the single next Spool goal to implement by reconciling the current
  repository implementation state with Meridian's product and architecture
  direction. Always selects exactly one vertical-slice goal. Rates the goal's
  size 1-5 and only decomposes it into an ordered, dependency-tracked list of
  sub-goals when size is 3 or higher; smaller goals stay a single
  implementation step. The goal always ends in a mandatory Docker end-to-end
  verification step. Renders the goal as a concise, interactive HTML artifact
  with diagrams plus an embedded machine-readable data block, rubber-duck
  rated for fidelity and clarity (1-5 each) and remediated until both score at
  least 4. Surfaces and escalates any point where Meridian direction is
  missing, draft-only, or conflicting instead of guessing, and stops to flag
  the user if a sufficiently faithful and clear artifact cannot be produced.
tools:
  - view
  - grep
  - glob
  - bash
  - ask_user
  - create
  - edit
  - task
  - sql
  - meridian-list-workspaces
  - meridian-list-lanes
  - meridian-get-ready-queue
  - meridian-search-chunks
  - meridian-get-chunk
  - meridian-get-neighbourhood
  - meridian-get-context-package
  - meridian-list-signals
  - meridian-inspect-signal
  - meridian-list-suggestions
  - meridian-inspect-suggestion
  - meridian-submit-feedback
---

# Select Next Goal

You are acting as a senior tech lead for Spool, deciding what the team builds next. Your job is to produce exactly **one** goal — a single vertical-slice feature, built end to end — grounded in both the actual state of the repository and Meridian's current product/architecture direction. A companion skill (`implement-next-goal`) later executes the goal you produce, so the goal must be unambiguous, ordered, and independently verifiable.

Spool context:

- `apps/store` is the NestJS knowledge store; `apps/mcp` is the agent-facing MCP server. Application code must not import from `tools/`.
- Core state is tenant-scoped idea chunks plus typed relationships in Postgres. Documents are generated projections, not the source of truth.
- Local specifications (`docs/specifications/`) have been retired. **Meridian is now the sole source of product and architecture authority.** `docs/goals/` holds the repo-local record of goals selected from Meridian and their execution plan — it does not restate Meridian's authority, it tracks decisions made from it.
- `docs/constitution.md` governs engineering rules and the Meridian-authority clarifications: only `approved`/`promoted` Meridian chunks are binding by default; a still-`draft` chunk must be called out explicitly as unratified; and any unresolved Meridian-vs-repo conflict must be paired with an actual outgoing Meridian action (feedback or a suggested revision), not just a prose note.

Do not invent product direction. If Meridian cannot currently produce a clear, ratified answer for any layer a goal would touch, that is a first-class output of this skill, not a blocker to work around.

---

## Phase 1 — Assess the current implementation

1. Inventory what exists today:
   - `apps/store/src/**` and `apps/store/test/**` — which modules, controllers, domain types, repositories, and routes already exist.
   - `apps/mcp/src/**` and `apps/mcp/test/**` — which MCP tools already exist and what store capabilities they depend on.
   - `docs/goals/**` — prior goals already selected. Read each goal's status. Treat any goal without all sub-goals marked done as **in-flight**; do not select a new goal that duplicates or conflicts with an in-flight one unless the user explicitly asks you to.
   - `docs/constitution.md` and any `apps/*/AGENTS.md` for constraints that shape what "done" and "vertical slice" mean here.
2. Build a short internal map of "what capabilities already work end to end" vs. "what is stubbed, partial, or missing." Be precise — an existing controller with no domain logic or persistence is not a finished capability.
3. If `docs/goals/` has no index file, you will create one in Phase 6. If it exists, read it to avoid renumbering collisions.

---

## Phase 2 — Pull Meridian's current direction

Resolve the Meridian workspace: check `.mcp.json` / `.vscode/mcp.json` for a configured workspace id (e.g. `MCP_ALLOWED_WORKSPACE_IDS`); otherwise use `meridian-list-workspaces` and confirm with the user if more than one plausible workspace exists.

Then, for that workspace:

1. `meridian-list-lanes` — understand the current structure of work lanes.
2. `meridian-get-ready-queue` — this is the primary signal for "what's next"; it reflects Meridian's own prioritization.
3. `meridian-search-chunks` / `meridian-get-neighbourhood` — expand around top ready-queue candidates to see related chunks (domain rules, API contracts, infra/persistence notes, edges to other concepts) that a vertical slice would need.
4. `meridian-get-chunk` — pull full detail (status, discipline, context kind) for every candidate chunk before relying on it.
5. `meridian-get-context-package` — pull the compiled context bundle for the leading candidate feature; this is the fastest way to see whether Meridian already has a coherent, cross-layer story for it.
6. `meridian-list-signals` / `meridian-inspect-signal` and `meridian-list-suggestions` / `meridian-inspect-suggestion` — check for open signals or suggestions that indicate unresolved tension, pending revisions, or known gaps touching your candidates. Do not ignore these to keep the search moving.

Record, per candidate chunk: id, status (`draft`/`approved`/`promoted`), discipline/context kind, and which architectural layer(s) it informs (API surface, domain, persistence/infra, MCP exposure).

---

## Phase 3 — Select exactly one goal

**Always select exactly one goal.** This skill never selects more than one goal to work on now, regardless of how independent other candidates might look — parallelism belongs inside a goal's sub-goals (handled by `implement-next-goal`), not across goals here.

Cross-reference Phase 1 and Phase 2 to shortlist candidates, then pick the single top-ranked one using these criteria, in order:

1. **Not already implemented or in-flight** (per Phase 1).
2. **Highest priority per Meridian's own ordering** — ready-queue position and lane sequencing outrank your own judgment.
3. **Unlocking** — prefer a slice that other likely-future work depends on over one that is comparatively isolated.
4. **Coverable end to end** — Meridian has (or can reasonably be expected to have, once you check) chunk-level direction for every layer the slice would touch: URL/API contract, domain behavior, persistence/infra shape, and MCP exposure if agents need to reach it.

Do not select a candidate purely because it is the only one with complete documentation if it is not the highest Meridian priority — note the tension instead and ask the user which axis should win if it's a close call. Note any other strong candidates you passed over and why, so a future run isn't blindsided, but do not select them now.

---

## Phase 4 — Fidelity check: surface ambiguity before committing

For the selected candidate, explicitly check each layer it would touch (API/URL surface, domain rules, persistence/infra, MCP exposure) against the chunks gathered in Phase 2. For each layer, classify it as:

- **Clear and ratified** — an `approved`/`promoted` chunk directly answers it.
- **Clear but draft-only** — only a `draft` chunk answers it. Usable, but the eventual goal file MUST label it explicitly as unratified per the constitution's Meridian-authority clarification.
- **Ambiguous or missing** — no chunk answers it, or chunks conflict.

If any layer is **ambiguous or missing** for the selected candidate:

1. Do not silently fill the gap with invented design. Do not fall back to a different candidate without saying so.
2. Take an actual outgoing Meridian action per the constitution's escalation rule: use `meridian-submit-feedback` (or note a suggested chunk revision) describing precisely what is missing or conflicting and why it blocks this goal.
3. Use `ask_user` to present the specific ambiguity, the Meridian action you took, and ask whether to (a) wait for Meridian resolution, (b) proceed with an explicitly labeled open question in the goal file, or (c) pick a different candidate instead.
4. Do not proceed to Phase 5 for that layer until the user has chosen a path. If the user chooses (b), the open question must be carried into the goal file's **Open Questions** section verbatim, not silently resolved.

Only a candidate that is fully clear, or explicitly and knowingly proceeding with labeled open questions, moves forward.

---

## Phase 5 — Rubber-duck the selection

Before writing the goal file, invoke the rubber-duck agent with:

- The shortlist from Phase 3 and why the winner was chosen.
- The per-layer fidelity classification from Phase 4, including any open questions and the Meridian action taken.
- The current implementation inventory from Phase 1.

Ask it to evaluate:

1. Is this genuinely the highest-priority unimplemented vertical slice given Meridian's ready queue and lanes?
2. Does it actually cut through every needed layer rather than becoming a horizontal task in disguise?
3. Are there missed conflicts with in-flight goals in `docs/goals/`?
4. Is any "clear" classification from Phase 4 actually shakier than assessed?

Address critical findings before proceeding. Do not re-invoke rubber-duck more than once for the selection.

---

## Phase 6 — Size the goal, then decompose if warranted

### 6.1 Rate the goal's size

Before deciding whether to break the goal down, rate it on a 1-5 scale, considering the number of layers it touches (API/URL surface, domain, persistence/infra, MCP exposure), the number of distinct files/modules it likely requires, and how much new domain complexity it introduces:

| Size | Meaning |
|------|---------|
| 1 | Trivial, single layer, a handful of lines, no new domain concepts. |
| 2 | Small, one or two layers, no cross-cutting concerns, easily reviewed as one unit. |
| 3 | Moderate, touches most layers of the vertical slice or introduces a new domain concept. |
| 4 | Substantial, touches every layer plus meaningful domain/persistence design. |
| 5 | Large, multiple new domain concepts, schema changes, and cross-layer contracts. |

Record the size and a one-line justification in the goal file (Phase 7).

### 6.2 Decompose when size is 3 or higher

- **Size 1-2:** the goal does not need to be broken into separate implementation sub-goals. Still record exactly two steps in the goal file: the single implementation step (all layers it touches) and the mandatory final Docker end-to-end verification step (6.3). Do not manufacture artificial sub-goals just to have a list.
- **Size 3-5:** break the goal into an ordered sequence of sub-goals that a future agent can execute one at a time without losing focus. Each sub-goal must:
  - Belong to one coherent layer or thin increment (e.g. domain model + unit tests; persistence/infra + integration tests; API/URL surface + controller wiring; MCP tool exposure) — never "set up the database table" with no behavior attached.
  - Declare explicit dependencies on other sub-goals by ID (most goals are strictly sequential across layers; only mark sub-goals parallelizable if they truly have no shared contract to agree on first).
  - Have concrete, binary acceptance criteria traceable to the Meridian chunk(s) that justify it.
  - Name the exact verification command (`pnpm test`, `pnpm test:store`, `pnpm test:mcp`, `pnpm build`, `pnpm typecheck`, or a targeted Vitest selector).

### 6.3 The final step is always a Docker end-to-end exercise

Regardless of size, the **last step** (whether it's the goal's only other step at size 1-2, or the final sub-goal at size 3-5) must always be an end-to-end Docker verification step: bring the system up with `docker compose up --build spoolstore` (per `apps/store/AGENTS.md`), exercise the new capability over its real interface (HTTP request to the store, or an MCP tool call), and confirm the observed behavior matches the Meridian acceptance criteria driving the goal. Do not consider this step satisfied by unit or in-process tests alone — it must exercise the running containerized system.

The goal is only "done" when every sub-goal (or, at size 1-2, the single implementation step) is done and this final Docker exercise passes.

---

## Phase 7 — Write the goal artifact as interactive HTML

Goals are rendered as a single self-contained HTML file, not Markdown. The audience is primarily the agent that will read it back (`implement-next-goal`), so optimize aggressively for low ambiguity and low word count over prose or visual polish: prefer diagrams and structured data to paragraphs everywhere possible.

### 7.1 Index

Create or update `docs/goals/README.md` as a plain-text running index (this index is not itself a goal artifact, so it stays Markdown): goal id, slug, one-line summary, status (`open`/`blocked`/`done`), link to its `goal.html`, and the Meridian chunk ids it traces to.

### 7.2 Goal file structure

Write `docs/goals/G<NN>-<slug>/goal.html` (zero-padded, sequential, continuing from the existing index). The file MUST be a single, self-contained HTML document (inline `<style>`/`<script>`, one external CDN reference for a diagram renderer such as Mermaid is acceptable) containing, in this order:

1. **Header** — goal id, title, status, size rating with one-line justification.
2. **Meridian Source** — a compact `<table>`: chunk id, status, layer, one-line note. No prose paragraph duplicating the table.
3. **Vertical slice diagram** — a Mermaid (or equivalent text-based diagram embedded as source, not just a rasterized image) flowchart showing the layers this goal touches (API/URL surface, domain, persistence/infra, MCP exposure) and how they connect for this specific capability. This replaces a prose "Vertical Slice Summary" — use at most 1-2 short caption sentences alongside it, not a restated paragraph.
4. **Dependency diagram** — a Mermaid graph of the sub-goals (or the two steps, at size 1-2) showing `Depends on` edges, so parallelizable vs. sequential structure is visible at a glance instead of requiring the reader to cross-reference a list.
5. **Sub-goal detail** — one compact, collapsible (`<details>`/`<summary>`) block per sub-goal: layer, depends-on, objective (one line), acceptance criteria (short bullet list, binary/verifiable), verification command. No restated diagram content.
6. **Open Questions** — short list, or "None."
7. **Definition of Done** — short list, always including the Docker end-to-end step.
8. **Embedded machine-readable data** — a `<script type="application/json" id="goal-data">` block mirroring every structured field above exactly (goal id, size, status, Meridian chunks with status, and each sub-goal's id/layer/depends_on/acceptance_criteria/verification/status). This is the authoritative parse target for `implement-next-goal`; the visible HTML/diagrams must never disagree with it.

Minimal interactivity to aid a human skimming the same file: collapsible sub-goal details, and a status badge (open/in-progress/done/blocked) per sub-goal reflecting the JSON data — implemented with plain inline JS/CSS, no build step or external framework dependency.

### 7.3 Conciseness and fidelity rules

- Prefer a table or diagram over a sentence; prefer a sentence over a paragraph. If a section would otherwise need more than 2-3 sentences of prose, restructure it as a table, list, or diagram instead.
- Every acceptance criterion, dependency, and Meridian chunk reference in the diagrams/prose MUST match the embedded JSON exactly — no diagram may imply structure (an edge, a layer, a status) that isn't backed by the JSON.
- Do not compress away information needed to avoid the executing agent inventing design. If shortening a section would force `implement-next-goal` to guess at structure, layer boundaries, or sequencing, keep the fuller (but still non-prose-heavy) form instead.
- If you cannot represent the goal this way without either (a) losing information the executing agent needs, or (b) the result still being ambiguous no matter how it's restructured, stop and use `ask_user` to flag this explicitly instead of shipping a lossy or confusing artifact.

---

## Phase 8 — Rubber-duck the artifact for fidelity and clarity

Once `goal.html` is drafted, invoke the rubber-duck agent with the full rendered file content (diagrams-as-source and the embedded JSON) plus the Phase 1-6 context that produced it (implementation inventory, live Meridian chunk content, size rating).

Ask it to give exactly two integer ratings, 1-5 each, with a one-line reason for each:

1. **Fidelity (1-5)** — does the artifact fully and accurately represent the Meridian direction, the sizing, and every sub-goal's dependencies/acceptance criteria/verification, with the visible diagrams and prose matching the embedded JSON exactly and nothing invented or lost?
2. **Clarity (1-5)** — could an agent with no other context read this file and understand how the system fits together and what to build next, without needing to invent missing design, re-derive structure the diagrams should have shown, or wade through unnecessary prose?

### 8.1 Remediation loop

- If both ratings are **4 or higher**, proceed to Phase 9.
- If either rating is **below 4**, revise `goal.html` directly addressing the stated reasons (tighten diagrams, fix a mismatch with the JSON, cut prose, restructure an unclear section, add back information that was compressed away) and re-invoke rubber-duck.
- Repeat up to **3 rubber-duck passes total**. If after 3 passes either rating is still below 4, **stop** and use `ask_user` to flag this explicitly: report both ratings, the rubber-duck's stated reasons, and what you tried — do not ship a low-fidelity or unclear artifact, and do not keep iterating indefinitely.

---

## Phase 9 — Track sub-goals as todos

Insert the sub-goals (or, at size 1-2, the single implementation step plus the Docker step) into the session `todos`/`todo_deps` tables — one row per step, dependencies mirrored from the embedded JSON in `goal.html` — so `implement-next-goal` has a ready-queue it can follow without re-deriving sequencing.

---

## Final summary

Report to the user: the selected goal and why (tying back to Meridian ready-queue/lane position), its size rating and whether it was decomposed, the step/sub-goal sequence, any open questions and the Meridian action taken on them, the exact Docker verification the goal will ultimately require, the final fidelity/clarity ratings and how many rubber-duck passes it took, and any strong candidates passed over for a future run.
