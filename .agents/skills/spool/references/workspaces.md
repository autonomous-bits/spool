# Bind and leftover `.spl`

Bind each code repository to a solution context remote:

```sh
spl context init --remote https://github.com/org/solution-context.git
```

That writes `.spool/context.toml` with `solution_id`, `remote`, and `protected_branch`. Context
management is bound-only. There is no `spl workspace` catalog and no Rack remote protocol.

Export leftover private `.spl` graph files into the bound context git:

```sh
spl context export
spl context migrate-once
```

Private `.spl` readers exist only for that export path. They are not public VCS commands.

`workspace init/attach`, `migrate`, `remote *`, `clone`, `push`, and `pull` are removed.
