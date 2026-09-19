# Merges

Always preview before applying. Preserve and reuse the preview ID and a caller-owned transaction ID.
This is a **file-graph** merge of two context-git branches, not a Rack/.spl lease.

```sh
spl merge preview --source feature --target main
spl merge apply --source feature --target main --transaction merge-42 \
  --preview <preview-id> --author alice --message "Merge feature"
```

Applying a clean, exact preview opens a short-lived branch and pull request. If conflicts exist,
inspect and resolve the cached merge state:

```sh
spl merge conflicts --transaction merge-42
spl merge resolve --transaction merge-42 --preview <preview-id> \
  --selections selections.json [--overrides mutations.json]
spl merge finalize --transaction merge-42
```

`selections.json` is a JSON array of conflict choices such as
`{"conflictId":"...","choice":"source"}` or `{"conflictId":"...","choice":"target"}`.
`--overrides` is an optional graph-mutation array for schema-derived semantic conflicts.

```sh
spl merge abort --transaction merge-42
```
