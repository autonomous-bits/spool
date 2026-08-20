# Merges

Always preview before applying. Preserve and reuse the preview ID and a caller-owned transaction ID.

```sh
spl merge preview --source feature --target main
spl merge apply --source feature --target main --transaction merge-42 \
  --preview <preview-id> --author alice --message "Merge feature"
```

Applying a clean, exact preview returns the merge commit. If conflicts exist, the target branch is
leased for the transaction; inspect and resolve it rather than retrying with a new preview:

```sh
spl merge conflicts --target main --transaction merge-42
spl merge resolve --target main --transaction merge-42 --preview <preview-id> \
  --selections selections.json [--overrides mutations.json]
spl merge finalize --target main --transaction merge-42
```

`selections.json` is a JSON array of conflict choices such as
`{"conflictId":"...","choice":"source"}` or `{"conflictId":"...","choice":"target"}`.
`--overrides` is an optional graph-mutation array for schema-derived semantic conflicts. To discard
a conflicted transaction without moving the target branch:

```sh
spl merge abort --target main --transaction merge-42
```

Do not alter the target branch outside this transaction while its merge lease exists.
