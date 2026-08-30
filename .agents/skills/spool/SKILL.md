---
name: spool
description: Use the local Spool graph version-control CLI (`spl`) to initialize repositories, stage and commit graph changes, manage branches and merges, query graph snapshots, validate schemas, and maintain repository integrity.
---

# Spool

Use `spl` from a Spool workspace. Successful commands emit JSON on stdout; errors are written to
stderr. Do not edit the resolved Spool state directory directly. Use `spl <command> --help` before
relying on flags or output fields not covered by these references.

## Branch Strategy & User Elicitation

Before staging or committing changes with `spl add`:
1. **Check the active branch**: Run `spl branch list` to identify the current branch and available branches.
2. **Elicit user intent**: Unless the user has already specified a target branch, prompt the user to clarify whether changes should be:
   - Committed directly to the current branch (e.g. `main`), or
   - Isolated on a new dedicated branch (e.g. `spl branch create <branch-name> --from-branch <current>`) to allow review, diffing (`spl diff`), and isolated merging (`spl merge`).
3. **Execute branch setup**: If a new branch was requested or agreed upon, create and switch to it before staging batches:
   ```sh
   spl branch create <new-branch> --from-branch <current-branch>
   spl switch <new-branch>
   ```

Before merging a feature branch containing transient planning data, preview its cleanup with
`spl prune --branch <branch> --dry-run`. After reviewing the JSON result, run `spl prune` to
commit removal of nodes labeled `Ephemeral` and their incident edges. `prune` refuses branches
with staged changes and requires `--force` for the protected default branch.

## Common CLI Invocation Rules & Pitfalls

1. **Native `spl` Queries Only**: Always use built-in `spl` commands (`filter`, `search`, `resolve`, `graph`, `context`, `diff`) directly. Do not pipe JSON outputs to Python, awk, or ad-hoc shell parsing scripts.
2. **`spl resolve`**: Requires `--node <node-id>` (e.g. `spl resolve --branch main --node <id>`). Running without `--node` fails.
3. **`spl diff`**: Uses `--base-branch <name>` and `--target-branch <name>` (do not use `--from` or `--to`).
4. **`spl merge apply`**: Requires `--preview <preview-id>` (obtained from `spl merge preview`) and `--transaction <tx-id>` along with `--source`, `--target`, `--author`, and `--message`.
5. **`spl context` & `spl search-expand`**:
   - Do **NOT** use `--id` or `--depth`.
   - Use `--query "<keywords>"` OR `--label <label>` (mutually exclusive with `--query`).
   - Use `--direction out|in|both` (default is `out`).
   - Use `--max-depth <int>` (not `--depth`), `--seed-limit <int>`, `--edge-type <type>`.
   - To inspect a single specific node by ID, use `spl resolve --branch main --node <id>`.

## Command index

| Commands | Reference |
| --- | --- |
| `init`, `add`, `status`, `commit` | [Working changes](references/working-changes.md) |
| Authoring `add` batches and atomic ideas | [Batch authoring](references/batch-authoring.md) |
| `branch`, `branch create`, `branch list`, `branch delete`, `switch`, `history`, `branches-containing`, `diff` | [Branches and history](references/branches-and-history.md) |
| `schema`, `schema migrate`, `validate` | [Schemas](references/schemas.md) |
| `resolve`, `search`, `filter`, `search-expand`, `context` | [Reading graphs](references/reading-graphs.md) |
| `merge`, `merge preview`, `merge apply`, `merge conflicts`, `merge resolve`, `merge finalize`, `merge abort` | [Merges](references/merges.md) |
| `fsck`, `gc`, `prune` | [Maintenance](references/maintenance.md) |
| `workspace`, `workspace init`, `workspace attach` | [Multi-repo workspaces](references/workspaces.md) |
| `completion`, `help` | [CLI help](references/cli-help.md) |
