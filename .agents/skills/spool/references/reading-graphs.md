# Reading graphs

All read commands write JSON and require a bound checkout (`.spool/context.toml`). `graph` exports
every node and edge. Retrieval commands (`search`, `filter`, `search-expand`, and `query-context`)
query the rebuilt projection.

```sh
spl resolve --node <node-id>
spl graph
spl search --query incident
spl filter --label Task --property-text status=open
spl filter --property-min priority=3
```

`filter` accepts repeatable `--label`, `--property-text key=value`, `--property-number key=value`,
`--property-min key=value`, and `--property-max key=value`. Filtered properties must be scalar and
enabled as indexed in the selected schema.

Build bounded graph context from either a lexical query or typed filters, never both:

```sh
spl search-expand --query incident --direction out --edge-type RELATES_TO
spl query-context --label Task --property-text status=open --direction both
```

The former query verb `context` is `query-context`. The `context` CLI namespace is only
init/export/migrate-once.

`query-context` and `search-expand` accept `--seed-limit`, repeatable `--edge-type`, and
`--direction out|in|both`.
