# Working changes

Bind this code repository to a solution context remote once:

```sh
spl context init --remote https://github.com/org/solution-context.git
```

That writes `.spool/context.toml`. Context-management commands refuse to run without it.

Export leftover private `.spl` graph files into the bound context remote (one-shot or repeatable):

```sh
spl context export
spl context migrate-once
```

These commands read leftover `.spl` internally. They are not Spool VCS commands and do not reopen
Rack or `.spl` as source of truth.

## Graph writes

There is no public `spl add` / `spl commit`. Routine node and edge writes use `spl mutate`
(MCP: `spl_mutate`). Bound writes open a short-lived `spool/mcp/<stamp>-<nonce>` branch and a
pull request:

```sh
spl mutate --batch mutations.json --message "Record requirement"
spl schema migrate --schema schema.toml --batch mutations.json
spl asset add --file docs/architecture.md --title "Architecture notes"
spl prune --author alice --message "Prune transient plan"
```

`mutate` is bound-only and refuses unbound workspaces. Use `schema migrate` when changing
`schema.toml`. Identical schema content is a no-op (no empty PR).

## History and diff

Use stock git on the context remote:

```sh
git log
git diff
```

Spool does not wrap `history` or `diff`.
