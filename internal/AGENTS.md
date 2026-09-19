# Internal packages

`internal/` is private to this module; do not add an `internal/spl/` layer.

See the [architecture guide](../docs/architecture.md) for the system-level
component boundaries, data model, and persistence design.

- `repository/` owns durable objects, refs, locking, recovery, and storage contracts.
- `repository/branch/` owns branch lifecycle requests, validation, and services.
- `repository/initialization/` owns repository initialization requests and service delegation.
- `repository/merge/` owns merge lifecycle requests, transactions, and services.
- `resolve/` owns node-resolution queries and their tool adapter; it is not repository storage.
- `contextual/` owns bounded evidence-and-expansion use cases over a pinned graph snapshot; it is
  not projection persistence.
- `workspace/` leftover detached-workspace provisioning is **deprecated** and
  unsupported for solution context; N code repos bind via `.spool/context.toml`.
- `ctxgit/` owns explicit `.spool/context.toml` bind resolution, stock-git
  context checkout, human-diffable layout, short-lived branch + PR writes,
  local projection rebuild, and one-shot `.spl` export. Git is the **only**
  durable solution-context source of truth.

Keep repository lifecycle behavior under `repository/`. Add other query use cases in appropriately named packages.
