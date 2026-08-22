# Evidence collection

## Implementation

- Run `git status --short --branch`; preserve all existing changes.
- Read repository guidance and architecture documentation, then inspect the
  relevant CLI/API, use-case, storage/domain, and focused tests.
- Inspect `docs/goals/*.html`; do not duplicate a goal already marked
  `planned`, `in_progress`, or `done`.

## Spool graph

Use the user-selected graph branch, or the default graph branch (normally
`main`) when none is named. Use explicit branch selectors and record the
returned snapshot commit.

```sh
spl branch list
spl status --branch <branch>
spl validate --branch <branch>
spl filter --branch <branch> --max-rows 200 \
  --max-response-bytes 1048576 --timeout 5s
```

`filter` is the supported graph inventory command; follow its continuation
token until complete. The installed CLI has no `graph` export command. Retrieve
candidate evidence with bounded context, then resolve every referenced node:

```sh
spl context --branch <branch> --query <term> --direction both \
  --max-depth 2 --max-visited 100 --max-rows 100 \
  --max-response-bytes 1048576 --timeout 5s
spl resolve --branch <branch> --node <node-id>
```

Use filters when the schema supports them. Treat graph nodes as evidence, not
proof that behavior is implemented; list only node IDs actually examined.

## Selection

Prefer the earliest prerequisite when a feature has dependencies. Select a
`blocked` goal only when recording its missing prerequisite is the useful
outcome. Do not infer node relationships from matching names alone.
