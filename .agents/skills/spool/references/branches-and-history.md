# Branches and history

History, diff, branch listing, and pull-request review use stock git and GitHub on the context remote:

```sh
git branch
git switch feature
git log -- nodes/ edges/
git diff main...feature -- nodes edges schema.toml
```

Review the pull request on the host. Spool writes open a short-lived `spool/mcp/<stamp>-<nonce>` branch and a pull request into the protected branch. Do not push the protected branch except during `spl context init`.

File-graph merge of two git revisions is a Spool KEEP command (`spl merge`, MCP `spl_merge_*`). See [Merges](merges.md).
