# Multi-repo workspaces

Create a central detached workspace, then bind each repository with a manifest:

```sh
spl workspace init ecommerce-platform
spl workspace attach --workspace ecommerce-platform --repository-id github.com/acme/order-service ~/repos/order-service
spl workspace attach --workspace ecommerce-platform --repository-id github.com/acme/inventory-service ~/repos/inventory-service
```

`workspace init` provisions detached state in the user's central catalog.
`workspace attach` writes a portable `.spl/config.toml` manifest binding the
checkout to the workspace's immutable ID; commit it for clones, worktrees, and
CI. Attachment does not persist or use a host-path association. There are no
active-workspace preferences or `SPOOL_WORKSPACE`; a checkout without a valid
manifest continues through normal local `.spl`/`go.work` discovery.
