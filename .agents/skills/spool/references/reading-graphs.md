# Reading graphs

All read commands write JSON. Prefer explicit branch selectors. `graph` exports every node and edge
from a branch snapshot. `resolve` can select a reachable
historical commit; retrieval commands (`search`, `filter`, `search-expand`, and `context`) query
the branch-head projection only, so historical commit selection is rejected.

```sh
spl resolve --branch main --node <node-id>
spl resolve --branch main --commit <commit-id> --node <node-id>
spl graph --branch main
spl search --branch main --query incident
spl filter --branch main --label Task --property-text status=open
spl filter --branch main --property-min priority=3
```

`filter` accepts repeatable `--label`, `--property-text key=value`, `--property-number key=value`,
`--property-min key=value`, and `--property-max key=value`. Filtered properties must be scalar and
enabled as indexed in the selected schema.

Build bounded graph context from either a lexical query or typed filters, never both:

```sh
spl search-expand --branch main --query incident --direction out --edge-type RELATES_TO
spl context --branch main --label Task --property-text status=open --direction both
```

For paged results, pass the returned `--continuation` token. Bound work and output with
`--max-rows`, `--max-response-bytes`, and `--timeout`; `resolve`, `search-expand`, and `context`
also support `--max-visited` and `--max-depth`. `search-expand` and `context` accept
`--seed-limit`, repeatable `--edge-type`, and `--direction out|in|both`.
