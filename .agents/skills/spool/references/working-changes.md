# Working changes

Solution context does **not** start with `spl init`. Bind the code repo and
write via MCP as a short-lived branch + PR:

```sh
spl context init --remote https://github.com/org/solution-context.git \
  --solution-id my-solution
```

See [docs/context-bind.md](../../../../docs/context-bind.md). `spl init` and
detached `workspace init` / `workspace attach` are **deprecated and
unsupported** for solution context. Leftover `.spl` is migration-only
(`spl context export`).

## Bound writes

Prefer MCP `spl_add` / `spl_commit`. One mutation batch becomes one git commit
on a short-lived branch and a pull request to `protected_branch`. Do not push
cleanly to the working or protected branch.

CLI fallback (bound workspace):

```sh
spl add --branch main --batch mutations.json
spl status --branch main
spl commit --branch main --author alice --message "Describe the graph change"
```

`add` validates and stages the entire batch; it does not commit. Bound
`commit` opens the PR. Use explicit `--branch` values, and inspect the JSON
result before using returned IDs in later commands.
