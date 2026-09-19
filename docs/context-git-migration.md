# One-shot `.spl` export to context git

This is the documented **escape hatch** from leftover `.spl`: a **best-effort, lossy, migrate-once** mapping into the solution context git remote. Leftover `.spl`/Rack is **unsupported** as a parallel SoT. This path is **not sync**, not dual-run, and not Rack wire-compat.

Entrypoints (never named “sync”):

```sh
spl context export --branch main
spl context migrate-once --branch main
```

MCP: `spl_context_export`

Re-run is **overwrite-at-own-risk**. There is no continuous migration and no dual-write window.

## Keep vs drop

| Keep (mapped into the context tree) | Drop (never copied; fail closed if they would land in the PR) |
| --- | --- |
| Nodes (`nodes/<id>.json`, repo-namespaced IDs) | Packs (`objects/pack`, `*.pack`) |
| Edges (`edges/<id>.json`) | Rack remotes (not copied, **not configured** as a side effect) |
| Schema (`schema.toml`) | Reflogs (`logs/`) |
| Assets (`assets/<hash>/…`, Git LFS at or above 512 KiB) | Merge leases (`merge/`) |
| | Projections (`graph.db`, `projection.db`; rebuild-on-start) |

Success output lists **kept vs skipped**. Lossy is OK and documented.

## Cutover (steps 1–7)

| Step | Action | Constraints |
| --- | --- | --- |
| 1 | **Create** an empty solution context git remote (GitHub/GitLab) | Stock git only; no Spool/Rack protocol |
| 2 | **`spl context init --remote <url>`** — layout/schema; seed `CodeRepository` | Layout via normal git; MCP writes still use short-lived branch + PR; never create a second local SoT |
| 3 | **Bind** each code repo via `.spool/context.toml` → the same context remote | Bind file is the only join key; MCP refuses writes until bound; exported IDs are **repo-namespaced** to match `CodeRepository` seeds |
| 4 | **One-shot export** `spl context export` / `spl_context_export` | Map graph → `nodes/` `edges/` `schema.toml` `assets/`; LFS on write; **one batch → one commit → PR**; do not copy packs/Rack remotes/reflogs/leases/projections |
| 5 | **Verify** against the checklist below; human reviews the export PR | Fail closed if Rack/pack/projection paths appear in the PR diff |
| 6 | **Cut over** — agents use MCP → branch+PR only; stop writing `.spl`/Rack | Single SoT flip after the export PR merges; **no dual-write window** |
| 7 | **Sunset** — Rack out of default install/docs; optional delete of local `.spl` after a confidence window | Deleting `.spl` is optional/irreversible without backup; projection caches may be wiped anytime (rebuild-on-start) |

Do not continue from old projection watermarks. After cutover, git history is the source of truth.

## Verify checklist

- [ ] Schema present (`schema.toml`) and validates sample nodes/edges
- [ ] Node/edge counts within expected ballpark (or deltas explained in the PR)
- [ ] Assets in-repo or LFS-pointered (≥512 KiB policy)
- [ ] No Rack remote / pack / projection / reflog / merge-lease files in the context repo
- [ ] Export PR open (or ready) — not a silent push to the protected branch
- [ ] Docs/output list **kept vs dropped** (honesty)

Export fails closed if any dropped class would land in the context tree or PR.

## Rollback / stuck mid-migrate

- **Before cutover:** keep `.spl` as a read-only backup; the context repo (or the export branch) can be discarded and re-exported.
- **After cutover:** git history is SoT — no automatic roll-forward from Rack; recovery is revert/fix PRs **or** restore a `.spl` backup and re-export (explicitly lossy, at-own-risk).
- **Partial export:** if the short-lived branch was pushed but the PR failed, delete or close that branch/PR and re-run at-own-risk. Do not merge a tree that contains forbidden paths.
- **Never** leave dual-SoT “temporary sync” as a supported state.

## Failure modes

| Failure | What to do |
| --- | --- |
| Unbound workspace (no `.spool/context.toml`) | Export refuses. Run `spl context init --remote` / write the bind file. |
| Forbidden path in the tree/PR | Export fails closed. Do not merge. Inspect the short-lived branch and delete it. |
| Missing asset blob | Node metadata is kept; the blob is listed as skipped (lossy). |
| Commit or PR open fails after files were written | Manual cleanup of the short-lived branch; re-run is overwrite-at-own-risk. |
| Re-run against an already-exported graph | Overlaps surface in the PR for human review; not continuous migration. |

## Non-goals

Full-fidelity Rack pack wire-compat; continuous sync from old remotes; dual-write during cutover; configuring Rack remotes; silent push to the protected branch.
