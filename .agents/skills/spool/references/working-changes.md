# Working changes

Initialize the resolved Spool state directory once:

```sh
spl init
```

For a workspace-backed repository, `spl workspace init` already initializes the workspace's state
when it is created, so no separate `spl init` step is needed after `spl workspace attach`. Without
a workspace, run `spl init` to create local `.spl` state at the nearest parent `.spl` or `go.work`
directory, or the current directory.

## Branch selection

Before staging changes, check the active branch (`spl branch list`) and elicit from the user whether
to work against the current branch or create a new branch:

```sh
# If creating a new branch
spl branch create feature-idea --from-branch main
spl switch feature-idea
```

## Staging and committing mutations

Stage a JSON array of graph-mutation operations, inspect the staged delta, then create an immutable
commit:

```sh
spl add --branch feature-idea --batch mutations.json
spl status --branch feature-idea
spl commit --branch feature-idea --author alice --message "Describe the graph change"
```

`add` validates and stages the entire batch for its branch; it does not commit. `commit` commits all
staged mutations for the named branch. Use explicit `--branch` values rather than assuming the
active branch, and inspect the JSON result before using returned IDs in later commands.

For the accepted mutation-array shape, use the installed CLI's `spl add --help` and repository
examples/tests. Do not invent or edit storage objects in the resolved Spool state directory.
