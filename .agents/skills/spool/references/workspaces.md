# Multi-repo workspaces

`spl workspace init` / `spl workspace attach` are **deprecated and
unsupported** for solution context. They are not how N code repos join one
context store.

Bind each code repo with `.spool/context.toml` (or `spl context init --remote`).
See [docs/context-bind.md](../../../../docs/context-bind.md). Leftover `.spl`
workspace manifests exist only so `spl context export` can migrate once.

## Leftover `.spl` format migration

```sh
spl migrate --from 1 --to 2
spl workspace migrate --from 1 --to 2
```

`migrate` upgrades leftover `.spl` state-directory format so export can still
read it. It is not dual-run and does not configure Rack remotes.
