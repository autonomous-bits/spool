# Maintenance

## Graph pruning

Remove temporary planning nodes labeled `Ephemeral` and their incident edges from the **bound** context checkout. This is graph cleanup. It does not collect git objects.

```sh
spl prune --dry-run
spl prune --author alice --message "Prune transient plan"
```

`prune` refuses a checkout without `.spool/context.toml`. Non-dry-run writes a short-lived branch and pull request. A zero-match run is an idempotent no-op. Object storage on the context remote is ordinary git.
