# Schemas

Author the desired schema in TOML, provide a complete conforming mutation batch, then atomically
stage both:

```sh
spl schema migrate --branch main --schema people.toml --batch people-mutations.json
spl commit --branch main --author alice --message "Migrate people schema"
```

`schema migrate` replaces that branch's staged set only after validating the candidate graph against
the target schema. The schema takes effect only when the staged set is committed.

Validate the selected immutable snapshot against its stored schema:

```sh
spl validate --branch main
spl validate --branch main --commit <commit-id>
```

The `--commit` value must be reachable from the named branch.
