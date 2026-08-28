---
name: spool-engineering-standards
description: Author and query engineering standards in Spool. Defines labels, atomic node structures, and relationship edges for coding conventions, security policies, API standards, testing requirements, and observability rules.
---

# Spool Engineering Standards

Use this skill when capturing, refining, or querying **engineering standards, coding conventions, and technical policies** in a Spool graph.

> [!IMPORTANT]
> **Knowledge Graph, Not Task Tracking**
> Engineering standards in Spool represent durable organizational invariants, rules, and best practices. They are **never** used for backlog tasks (e.g. "Add linter to CI", "Write unit tests", "Fix security scan"). Every node must be an atomic statement of policy or rule.

---

## 1. Domain Taxonomy & Labels

Every engineering standard node should include the primary `Standard` label alongside one or more specific classification labels:

| Label | Purpose | Example Title |
| --- | --- | --- |
| `Standard` | Top-level domain tag applied to all engineering standards nodes | *(Used with sub-labels)* |
| `Convention` / `Guideline` | A code style, naming, error handling, or structural rule | `Go packages must not expose mutable global state or init() side-effects` |
| `SecurityPolicy` | A mandatory security guardrail, credential rule, or encryption standard | `API tokens must be stored using SHA-256 hashing with per-tenant salting` |
| `TestingStandard` | Testing expectations, coverage rules, or verification invariants | `All public API handlers must include hermetic end-to-end integration tests` |
| `ObservabilityStandard` | Telemetry, tracing, structured logging, or metric naming rules | `Log messages must be structured JSON containing trace_id and span_id fields` |
| `APIStandard` | HTTP/REST, gRPC, or messaging interface standards | `All HTTP 4xx and 5xx responses must conform to RFC 7807 Problem Details schema` |
| `DeploymentStandard` | Zero-downtime migration, release, or rollback policy | `Database schema migrations must be backward-compatible with the prior application version` |
| `AntiPattern` | An explicitly forbidden pattern with documented hazards | `Direct cross-service database queries bypassing domain API boundaries` |

---

## 2. Standard Edge Relationships

Link atomic engineering standards using explicit, semantic edge types:

| Edge Type | Source Node | Target Node | Meaning |
| --- | --- | --- | --- |
| `GOVERNS` | `Standard` | `Component` *(Architecture)* | The standard applies to and governs the specified component/service. |
| `FORBIDS` | `Standard` | `AntiPattern` | The standard explicitly proscribes this anti-pattern. |
| `COMPLIES_WITH` | `Decision` *(Architecture)* | `Standard` | The architectural decision is aligned with and honors the standard. |
| `SUPERSEDES` | `Standard` | `Standard` | A modern standard replaces a legacy convention. |
| `CONFLICTS_WITH` | `Standard` | `Standard` | Conflicting policies or contradictory conventions across teams. |

---

## 3. Node Content & Title Guidelines

### Atomic, Self-Contained Titles
State the rule, policy, or convention clearly with its scope and invariant:

- **Good**: `HTTP error payloads must adhere to RFC 7807 problem details with type, title, and instance fields`
- **Bad**: `Error handling` *(Vague, uninformative)*
- **Bad**: `Update endpoints to return JSON errors` *(Task-oriented)*
- **Bad**: `Logging and tracing guidelines for Go microservices` *(Combines multiple domains)*

### Anti-Patterns to Avoid
1. **Task Language**: Do not author "Run linter on commit" or "Format code with prettier". State the invariant: `Source code formatting in CI is enforced via golangci-lint with zero tolerated warnings`.
2. **Abstract Platitudes**: Avoid vague rules like "Write clean code". State concrete standards like `Functions must not exceed 50 lines of code excluding test fixtures`.
3. **Implicit Anti-Patterns**: When banning an unsafe technique, create an `AntiPattern` node and link it from the `Standard` via a `FORBIDS` edge so reasoning agents understand both the rule and what to avoid.

---

## 4. Batch Authoring Example

Create an `spl add` batch JSON file:

```json
[
  {
    "action": "add",
    "entity": "node",
    "id": "std-rfc7807-errors",
    "title": "All public HTTP APIs return error responses formatted according to RFC 7807 Problem Details",
    "labels": ["Standard", "APIStandard"],
    "properties": {
      "mandatory": {"kind": "bool", "bool": true},
      "contentType": {"kind": "string", "string": "application/problem+json"}
    }
  },
  {
    "action": "add",
    "entity": "node",
    "id": "antipattern-generic-500-string",
    "title": "Returning plain text 500 Internal Server Error strings without structured error codes",
    "labels": ["Standard", "AntiPattern"],
    "properties": {
      "hazard": {"kind": "string", "string": "Clients cannot reliably automate error handling and retry decisions"}
    }
  },
  {
    "action": "add",
    "entity": "edge",
    "id": "edge-rfc7807-forbids-plain-500",
    "source": "std-rfc7807-errors",
    "target": "antipattern-generic-500-string",
    "type": "FORBIDS"
  },
  {
    "action": "add",
    "entity": "edge",
    "id": "edge-rfc7807-governs-order-service",
    "source": "std-rfc7807-errors",
    "target": "comp-order-service",
    "type": "GOVERNS"
  }
]
```

Stage and commit the batch:

```sh
spl add --branch main --batch standards-batch.json
spl status --branch main
spl commit --branch main --author "Staff Engineer <standards@example.com>" --message "Record RFC 7807 error standard and anti-pattern"
```

---

## 5. Querying Engineering Standards

Discover and traverse engineering standards using `spl`:

```sh
# Search for standards relating to security or error handling
spl search --branch main --query "error RFC 7807"

# Filter all active security policies
spl filter --branch main --label SecurityPolicy

# Find all forbidden anti-patterns in the organization
spl filter --branch main --label AntiPattern

# Resolve a specific engineering standard or component by its exact ID
spl resolve --branch main --node comp-order-service

# Inspect standards and components around a specific topic
spl context --branch main --query "RFC 7807" --direction both --max-depth 2
```
