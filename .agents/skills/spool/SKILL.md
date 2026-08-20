---
name: spool
description: Use the local Spool graph version-control CLI (`spl`) to initialize repositories, stage and commit graph changes, manage branches and merges, query graph snapshots, validate schemas, and maintain repository integrity.
---

# Spool

Use `spl` from a Spool workspace. Successful commands emit JSON on stdout; errors are written to
stderr. Do not edit `.spl/` directly. Use `spl <command> --help` before relying on flags or output
fields not covered by these references.

## Command index

| Commands | Reference |
| --- | --- |
| `init`, `add`, `status`, `commit` | [Working changes](references/working-changes.md) |
| Authoring `add` batches and atomic ideas | [Batch authoring](references/batch-authoring.md) |
| `branch`, `branch create`, `branch list`, `branch delete`, `switch`, `history`, `branches-containing`, `diff` | [Branches and history](references/branches-and-history.md) |
| `schema`, `schema migrate`, `validate` | [Schemas](references/schemas.md) |
| `resolve`, `search`, `filter`, `search-expand`, `context` | [Reading graphs](references/reading-graphs.md) |
| `merge`, `merge preview`, `merge apply`, `merge conflicts`, `merge resolve`, `merge finalize`, `merge abort` | [Merges](references/merges.md) |
| `fsck`, `gc` | [Maintenance](references/maintenance.md) |
| `completion`, `help` | [CLI help](references/cli-help.md) |
