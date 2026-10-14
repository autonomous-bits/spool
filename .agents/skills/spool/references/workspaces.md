# Bind and one-shot export

Bind each code repository to a solution context remote:

```sh
spl context init --remote https://github.com/org/solution-context.git
```

That writes `.spool/context.toml` with `solution_id`, `remote`, and `protected_branch`. Context management is bound-only.

If you still have a leftover `.spl` store, run `context export` / `migrate-once` once into the bound context git remote, then stop using `.spl`. Export is lossy; re-run is overwrite-at-own-risk.

```sh
spl context export
spl context migrate-once
```
