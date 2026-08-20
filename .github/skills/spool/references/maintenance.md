# Maintenance

Check integrity after interruption, storage failure, or suspected corruption:

```sh
spl fsck
```

`fsck` is read-only, emits its JSON report even when corruption is found, exits non-zero for
corruption, and does not repair data automatically.

Pack retained objects and prune only unreachable loose objects beyond the grace period:

```sh
spl gc
spl gc --dry-run
spl gc --repack --grace-period 336h
```

Run `--dry-run` before maintenance when its effect is uncertain. `gc` retains branch, reflog, and
durable merge-resolution roots.
