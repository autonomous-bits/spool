# Working changes

Bind this code repository to a solution context remote once:

```sh
spl context init --remote https://github.com/org/solution-context.git
```

That writes `.spool/context.toml`. Context-management commands refuse to run without it.

If you still have a leftover `.spl` store, run `context export` / `migrate-once` once into the bound context git remote, then stop using `.spl`. Export is lossy; re-run is overwrite-at-own-risk.

```sh
spl context export
spl context migrate-once
```

## Graph writes

Routine node and edge writes use `spl mutate` (MCP: `spl_mutate`). Bound writes open a short-lived `spool/mcp/<stamp>-<nonce>` branch and a pull request:

```sh
spl mutate --operations mutations.json --message "Record requirement"
spl schema migrate --schema schema.toml --batch mutations.json
spl asset add --file docs/architecture.md --title "Architecture notes"
spl prune --author alice --message "Prune transient plan"
```

`mutate` is bound-only and refuses a checkout without `.spool/context.toml`. Use `schema migrate` when changing `schema.toml`. Identical mutate or schema content is a no-op (no empty PR).

## History, diff, and review

Use stock git and GitHub on the context remote:

```sh
git log
git diff
```

Review the pull request on the host.
