# Batch authoring

An `spl add` batch is a JSON array. Create the file, stage it, inspect the result, and commit only
when the staged delta is correct:

```json
[
  {
    "action": "add",
    "entity": "node",
    "id": "rate-limit-observation",
    "title": "The API returns 429 after 100 requests per minute",
    "labels": ["Observation"],
    "properties": {
      "limit": {"kind": "integer", "integer": 100}
    }
  }
]
```

```sh
spl add --branch main --batch mutations.json
spl status --branch main
spl commit --branch main --author alice --message "Record rate limit"
```

Each node represents one atomic idea: one fact, decision, requirement, question, or task. Its title
must make sense without relying on another node, conversation, or document. Put relationships
between ideas in edges rather than combining ideas into one node.

| Good atomic, self-contained title | Bad title | Why the bad title fails |
| --- | --- | --- |
| `The API returns 429 after 100 requests per minute` | `Rate limits and retries` | Combines an observation and a proposed response. |
| `Retry HTTP 429 responses with exponential backoff` | `Make the API reliable` | Too broad; it contains many possible changes. |
| `Password reset links expire after 30 minutes` | `Fix password reset and email templates` | Combines independent work items. |
| `Should audit logs retain IP addresses for 30 days?` | `Security questions` | Does not state a single answerable question. |

Use stable, descriptive IDs. Add edges when a relationship matters:

```json
[
  {
    "action": "add",
    "entity": "node",
    "id": "retry-429",
    "title": "Retry HTTP 429 responses with exponential backoff",
    "labels": ["Requirement"]
  },
  {
    "action": "add",
    "entity": "edge",
    "id": "retry-429-addresses-rate-limit",
    "source": "retry-429",
    "target": "rate-limit-observation",
    "type": "ADDRESSES"
  }
]
```

Properties are typed values: `null`, `bool`, `integer`, `float`, `string`, `list`, or `map`.
`list` and `map` values recursively contain typed values. Use `spl add --help` for the current
operation contract before authoring unfamiliar fields.
