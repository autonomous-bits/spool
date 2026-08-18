# Internal packages

`internal/` is private to this module; do not add an `internal/spl/` layer.

See the [architecture guide](../docs/architecture.md) for the system-level
component boundaries, data model, and persistence design.

- `repository/` owns durable objects, refs, locking, recovery, and storage contracts.
- `repository/branch/` owns branch lifecycle requests, validation, and services.
- `repository/initialization/` owns repository initialization requests and service delegation.
- `repository/merge/` owns merge lifecycle requests, transactions, and services.
- `resolve/` owns node-resolution queries and their tool adapter; it is not repository storage.

Keep repository lifecycle behavior under `repository/`. Add other query use cases in appropriately named packages.
