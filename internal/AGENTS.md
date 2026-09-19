# Internal packages

`internal/` is private to this module; do not add an `internal/spl/` layer.

See the [architecture guide](../docs/architecture.md) for the system-level
component boundaries, data model, and persistence design.

- `ctxgit/` is the public durable solution-context path: bind file, checkout, file graph,
  disposable projection, short-lived branch + PR writes (including `mutate`), bound queries,
  file-graph merge, graph prune, leftover `.spl` export.
- `repository/` is a **private library** (CAS/Rack leftovers). Do not wrap it as public CLI/MCP.
  Private readers are allowed only for `context export`.
- `resolve/` owns query-budget and retrieval result shapes used by bound ctxgit reads.
- `contextual/` owns bounded evidence-and-expansion types used by `query-context` / `search-expand`.
- `workspace/` and `remote/` remain private libraries. They are not public commands.

New public lifecycle behavior belongs on `ctxgit/`. Do not reopen Rack or `.spl` as source of truth.
