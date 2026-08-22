# Working changes

Initialize the resolved Spool state directory once:

```sh
spl init
```

For an attached workspace, initialize it after the first `spl workspace attach`; later attachments
share the same state and do not need another `spl init`. Without a workspace, this creates local
`.spl` state at the nearest parent `.spl` or `go.work` directory, or the current directory.

Stage a JSON array of graph-mutation operations, inspect the staged delta, then create an immutable
commit:

```sh
spl add --branch main --batch mutations.json
spl status --branch main
spl commit --branch main --author alice --message "Describe the graph change"
```

`add` validates and stages the entire batch for its branch; it does not commit. `commit` commits all
staged mutations for the named branch. Use explicit `--branch` values rather than assuming the
active branch, and inspect the JSON result before using returned IDs in later commands.

For the accepted mutation-array shape, use the installed CLI's `spl add --help` and repository
examples/tests. Do not invent or edit storage objects in the resolved Spool state directory.
