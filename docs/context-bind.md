# Binding code repos to a solution context remote

Spool’s happy path is **N code repositories bound to one solution context git remote**. Git is the durable source of truth for shared context. Spool MCP/CLI validates mutations, writes human-diffable files, opens a short-lived branch and pull request, and rebuilds a **local** query projection. `.spl` and Rack remotes are not that source of truth.

## Bind file

Each code repo that participates in a solution must contain an **explicit** bind file at `.spool/context.toml`. Spool never infers the context remote from directory names, monorepo layout, `go.work`, leftover `.spl` state, git `origin` URLs, or Rack config.

```toml
solution_id = "my-solution"
remote = "https://github.com/org/my-solution-context.git"
protected_branch = "main"
```

| Field | Required | Meaning |
| --- | --- | --- |
| `solution_id` | yes | Stable identifier for the solution whose context this remote holds |
| `remote` | yes | Stock git URL of the **shared** context repository (HTTPS or SSH) |
| `protected_branch` | no | Integration branch agents open PRs against (default `main`) |
| `repository_id` | no | Namespace for node IDs from this code repo (default: directory name of the bind file’s code root) |
| `lfs_threshold_kib` | no | Git LFS cutoff for non-text assets in KiB (default `512`) |

Unbound workspaces refuse graph mutations. There is no auto-discovery fallback.

## Multi-repo bind

One context remote per solution. Every code repo points at the **same** `remote` and `solution_id`, with a distinct `repository_id`:

```text
svc-api/.spool/context.toml    → remote = https://github.com/org/checkout-context.git
svc-web/.spool/context.toml    → remote = https://github.com/org/checkout-context.git
infra/.spool/context.toml      → remote = https://github.com/org/checkout-context.git
```

The graph is solution-flat: cross-repo edges are first-class. Provenance is repo-namespaced node IDs plus `CodeRepository` nodes — not path-siloed `repos/<name>/…` trees.

## Onboarding

From each code repo:

```sh
spl context init --remote https://github.com/org/my-solution-context.git \
  --solution-id my-solution \
  --repository-id github.com/org/svc-api
```

This command:

1. Writes `.spool/context.toml` in the current code repo (only).
2. Ensures the context remote is reachable with **stock git**.
3. Creates `schema.toml`, `nodes/`, `edges/`, `assets/`, and `README.md` when the remote is empty.
4. Seeds a `CodeRepository` node from **this** bind. Sibling directories are not scanned.
5. Leaves the workspace ready for MCP writes (short-lived branch + PR).

Repeat in every other code repo with the same `--remote` and `--solution-id`. Each init seeds that repo’s `CodeRepository` without discovering the others.

Existing `.spl` graphs move with a **one-shot** export (`spl context export` / `spl context migrate-once` / MCP `spl_context_export`). That path is lossy migrate-once, not sync. See [context-git-migration.md](context-git-migration.md).

## Context history (stock git)

Context history is ordinary git/GitHub:

- `git clone` / `git fetch` the bind `remote`
- Agents write via MCP: one mutation batch → one commit on a short-lived branch → pull request to `protected_branch`
- Humans review overlapping edits on the PR; there is no silent overwrite and no Spool-specific remote protocol

Local SQLite/FTS projections rebuild from the current checkout on every MCP/process start. They are never committed, pushed, or treated as a second store.

## What is not the happy path

These remain for local graph-VCS and migration, but they are **not** the durable solution-context store:

- `spl init` / `.spl` object packs
- `spl workspace init` / `workspace attach`
- `spl remote set`, `spl push`, `spl pull`, `spl clone` against Rack

When a bind file is present, the CLI refuses those commands and points at `.spool/context.toml` plus stock git. MCP tools for Rack remotes and detached workspaces always fail closed with the same guidance.
