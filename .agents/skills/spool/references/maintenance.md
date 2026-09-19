# Maintenance

## Graph pruning

Remove temporary planning nodes labeled `Ephemeral` and their incident edges from the **bound**
context checkout. This is graph cleanup, not pack/CAS garbage collection.

```sh
spl prune --dry-run
spl prune --author alice --message "Prune transient plan"
```

`prune` refuses unbound workspaces. Non-dry-run writes a short-lived branch and pull request.
A zero-match run is an idempotent no-op.

`fsck`, `gc`, and pack prune are **removed**. Do not call them. Object storage on the context
remote is ordinary git.
