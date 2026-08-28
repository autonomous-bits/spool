# Maintenance

Check integrity after interruption, storage failure, or suspected corruption:

```sh
spl fsck
```

`fsck` is read-only, emits its JSON report even when corruption is found, exits non-zero for
corruption, and does not repair data automatically.

## Graph pruning

Remove temporary planning nodes labeled `Ephemeral` and their incident edges from a branch before
merging it into a baseline:

```sh
# Review the affected nodes, cascading edges, and newly disconnected durable nodes.
spl prune --branch feature --dry-run

# Create a pruning commit after reviewing the preview.
spl prune --branch feature --author alice --message "Prune transient plan"
```

`prune` emits a JSON result with the pruning commit, counts, removed node IDs, and durable nodes
left without connected edges. It requires `--branch` and refuses a branch with staged changes.
Pruning the protected default branch additionally requires `--force`.

This graph operation is separate from `gc`: `prune` removes graph entities intentionally, while
`gc` collects unreachable immutable storage objects.

## Object garbage collection

Pack retained objects and prune only unreachable loose objects beyond the grace period:

```sh
spl gc
spl gc --dry-run
spl gc --repack --grace-period 336h
```

Run `--dry-run` before maintenance when its effect is uncertain. `gc` retains branch, reflog, and
durable merge-resolution roots.
