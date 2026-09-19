# Branches and history

Spool does not wrap git branch, switch, history, or diff. Use stock git on the context remote:

```sh
git branch
git switch feature
git log -- nodes/ edges/
git diff main...feature -- nodes edges schema.toml
```

MCP writes still create short-lived `spool/mcp/<stamp>-<nonce>` branches and open pull requests
into the protected branch. Do not push the protected branch except during `spl context init`.

File-graph merge of two git revisions remains a Spool KEEP command (`spl merge *`). See
[Merges](merges.md).

`cherry-pick`, `history`, `diff`, `branches-containing`, `branch`, and `switch` are removed.
