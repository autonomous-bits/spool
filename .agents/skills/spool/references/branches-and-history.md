# Branches and history

Create a branch from exactly one existing branch or commit, list branches, delete an inactive
non-default branch, or switch the active branch:

```sh
spl branch create feature --from-branch main
spl branch create review --from-commit <commit-id>
spl branch list
spl switch feature
spl branch delete feature
```

Trace commits that changed an entity, including all merge parents when needed:

```sh
spl history --branch main --entity-id <node-id>
spl history --branch main --commit <commit-id> --entity-id <node-id> --all-parents
```

Find branches using one selector: an entity ID, exact snapshot ID, or schema-defined natural key.

```sh
spl branches-containing --entity-id <node-id>
spl branches-containing --snapshot-id <snapshot-id>
spl branches-containing --natural-key <key>
```

Compare two snapshots selected by branch, optionally pinning either side to a reachable commit:

```sh
spl diff --base-branch main --target-branch feature
spl diff --base-branch main --base-commit <base-id> \
  --target-branch feature --target-commit <target-id> --max-rows 100
```

Use `--node-id`, `--edge-id`, `--node-title-contains`, `--one-hop`, `--continuation`,
`--max-rows`, and `--max-response-bytes` to constrain `diff`. Use `--continuation`,
`--max-rows`, `--max-response-bytes`, and `--timeout` to bound `branches-containing`.
