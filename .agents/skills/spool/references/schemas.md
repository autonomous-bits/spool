# Schemas

Author the desired schema in TOML, optionally provide a conforming mutation batch, and write both
through a short-lived branch and pull request:

```sh
spl schema migrate --schema people.toml --batch people-mutations.json
spl validate
```

`schema migrate` validates the candidate graph against the target schema before opening a PR.
Identical schema content is a no-op (no empty PR). Routine node/edge writes without a schema change
use `spl mutate` / `spl_mutate`. There is no public `spl add` / `spl commit`.

`validate` checks the bound checkout against `schema.toml`.
