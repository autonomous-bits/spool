---
name: spool-technical-implementation
description: Author, synthesize, and query technical implementation knowledge in Spool. Synthesizes cross-role atomic ideas (Product, Architecture, Standards) into technical execution specs, marks transient spec nodes as Ephemeral, and treats the implemented codebase as the ultimate source of truth.
---

# Spool Technical Implementation

Use this skill when preparing, designing, authoring, or executing **technical implementation specifications, execution plans, and code-level technical tasks** in a Spool workspace.

> [!IMPORTANT]
> **Core Implementation Invariants**
> 1. **Code is the Living Source of Truth**: The codebase (source code, tests, types, configs) is the definitive source of truth for technical execution and behavior. Technical specifications exist to align and guide development, but they do not replace or outlive code as the long-term truth of *how* the system works.
> 2. **Ephemeral Node Tagging**: All atomic ideas generated for technical implementation details, step-by-step execution plans, and transient technical specs must be tagged with the **`Ephemeral`** label (`labels: ["Implementation", "Ephemeral", ...]`). Do **not** create permanent graph nodes for transient implementation details that will be embodied directly in code.
> 3. **Preserve Long-Lived Concepts**: Only concepts that are truly long-lived and will not be replaced by code (such as new architectural decisions, permanent schema contracts, system boundaries, or organization-wide standards) should be authored as permanent, non-ephemeral nodes using the appropriate role skills ([`spool-architecture`](../spool-architecture/SKILL.md) or [`spool-engineering-standards`](../spool-engineering-standards/SKILL.md)).
> 4. **Cross-Role Context Synthesis**: Technical implementation must take into account all upstream roles in the Spool graph relevant to the vertical goal: **Product** requirements, **Architecture** decisions & topologies, and **Engineering Standards** (security, testing, API, coding rules).

---

## 1. Domain Taxonomy & Labels

### Implementation Labels
| Label | Purpose | Example Title |
| --- | --- | --- |
| `Implementation` | Top-level domain tag applied to all technical implementation nodes | *(Used with sub-labels)* |
| `TechnicalSpec` | High-level technical execution design or component specification | `Implement atomic batch staging and commit rollback in repository engine` |
| `Plan` | Structured sequence of technical execution phases or tasks | `Multi-phase rollout plan for atomic workspace directory migration` |
| `Ephemeral` | **Mandatory tag** for transient spec/plan nodes that will be superseded by code | *(Applied alongside `Implementation` / `TechnicalSpec` / `Plan`)* |
| `Spike` | Time-boxed technical exploration or prototype finding | `Spike findings on SQLite WAL mode concurrency under concurrent readers` |
| `Task` / `Step` | Atomic technical execution unit during active development | `Add lock acquisition timeout handler to repository mutex wrapper` |

### Upstream Scoped Labels (Cross-Role Synthesis)
Technical implementation actively references and connects to nodes from all other domains:

| Domain | Referenced Labels | Purpose in Implementation Context |
| --- | --- | --- |
| **Product** | `Product`, `Requirement`, `Problem`, `Capability`, `Constraint` | What user outcome, business rule, or constraint the technical change satisfies. |
| **Architecture** | `Architecture`, `Decision`, `ADR`, `Component`, `Boundary`, `DataContract`, `QualityAttribute`, `Tradeoff` | Structural decisions, component boundaries, latency/memory invariants, and contracts to follow. |
| **Engineering Standards** | `Standard`, `Convention`, `SecurityPolicy`, `TestingStandard`, `APIStandard`, `AntiPattern` | Organizational coding policies, forbidden anti-patterns, security rules, and testing standards to obey. |

---

## 2. Standard Edge Relationships

Connect technical implementation nodes to upstream role nodes and dependent execution steps:

| Edge Type | Source Node | Target Node | Meaning |
| --- | --- | --- | --- |
| `IMPLEMENTS` | `TechnicalSpec` / `Implementation` | `Requirement` *(Product)* | The technical work implements the specified business requirement. |
| `REALIZES` | `TechnicalSpec` / `Implementation` | `Decision` *(Architecture)* | The technical work realizes or executes the architectural decision. |
| `COMPLIES_WITH` | `TechnicalSpec` / `Implementation` | `Standard` / `SecurityPolicy` / `TestingStandard` | The technical implementation conforms to the engineering standard or policy. |
| `TARGETS` | `TechnicalSpec` / `Implementation` | `Component` *(Architecture)* | The implementation modifies or belongs to the specified system component. |
| `BLOCKED_BY` | `Task` / `Step` | `Task` / `Step` | Sequential ordering between execution steps during an implementation. |
| `SUPERSEDES` | `TechnicalSpec` | `TechnicalSpec` | A revised implementation spec replaces a prior approach. |

---

## 3. Implementation Workflow

```mermaid
flowchart TD
    A["1. Select Goal & Scope"] --> B["2. Query Cross-Role Context (Product, Architecture, Standards)"]
    B --> C["3. Synthesize Technical Spec (Mark as Ephemeral)"]
    C --> D["4. Implement in Code (Codebase is Source of Truth)"]
    D --> E["5. Verify (Tests, Lint, Standards Compliance)"]
    E --> F{"New Durable Concepts Discovered?"}
    F -- Yes --> G["Author Permanent ADR / Standard / Contract Nodes"]
    F -- No --> H["Commit Code & Complete Goal"]
    G --> H
```

### Step 1: Cross-Role Context Gathering
Before writing technical specs or code, query the Spool graph across all roles for relevant constraints and context:

```sh
# 1. Search for relevant product requirements and constraints
spl search --branch main --query "<feature-keyword>"

# 2. Inspect architectural context (decisions, components, contracts)
spl context --branch main --query "<component-or-feature>" --direction both --max-depth 2

# 3. Check engineering standards, security policies, and anti-patterns
spl filter --branch main --label SecurityPolicy
spl filter --branch main --label TestingStandard
spl filter --branch main --label AntiPattern
```

### Step 2: Formulate Technical Spec & Execution Plan
Draft atomic implementation nodes and connect them to upstream product requirements, architecture decisions, and standards.

> [!CAUTION]
> Always include `Ephemeral` on transient implementation nodes. Do not pollute the permanent graph with ephemeral code-level details.

### Step 3: Implement in Code
Implement the changes in the codebase. The code, types, and automated tests are the primary source of truth.

### Step 4: Promote Durable Knowledge (If Any)
If the implementation uncovered a new architectural invariant or reusable standard:
- Create a permanent `Decision` / `ADR` node with `spool-architecture`.
- Create a permanent `Standard` / `SecurityPolicy` node with `spool-engineering-standards`.
- Leave transient implementation steps marked as `Ephemeral`.

---

## 4. Batch Authoring Example

Create an `spl add` batch JSON file demonstrating cross-role synthesis and `Ephemeral` labeling:

```json
[
  {
    "action": "add",
    "entity": "node",
    "id": "spec-tx-outbox-relay-worker",
    "title": "Implement transactional outbox relay worker polling outbox table every 200ms with batch Kafka dispatch",
    "labels": ["Implementation", "TechnicalSpec", "Ephemeral"],
    "properties": {
      "pollIntervalMs": {"kind": "integer", "integer": 200},
      "batchSize": {"kind": "integer", "integer": 100},
      "status": {"kind": "string", "string": "in_progress"}
    }
  },
  {
    "action": "add",
    "entity": "node",
    "id": "task-outbox-relay-metrics",
    "title": "Add Prometheus counters for relay worker dispatched event count and batch dispatch latency",
    "labels": ["Implementation", "Task", "Ephemeral"],
    "properties": {
      "metricPrefix": {"kind": "string", "string": "outbox_relay"}
    }
  },
  {
    "action": "add",
    "entity": "edge",
    "id": "edge-spec-implements-product-req",
    "source": "spec-tx-outbox-relay-worker",
    "target": "req-deferred-billing-address",
    "type": "IMPLEMENTS"
  },
  {
    "action": "add",
    "entity": "edge",
    "id": "edge-spec-realizes-arch-adr",
    "source": "spec-tx-outbox-relay-worker",
    "target": "adr-order-outbox-kafka",
    "type": "REALIZES"
  },
  {
    "action": "add",
    "entity": "edge",
    "id": "edge-spec-complies-with-observability-std",
    "source": "task-outbox-relay-metrics",
    "target": "std-structured-json-logging",
    "type": "COMPLIES_WITH"
  },
  {
    "action": "add",
    "entity": "edge",
    "id": "edge-spec-targets-order-service",
    "source": "spec-tx-outbox-relay-worker",
    "target": "comp-order-service",
    "type": "TARGETS"
  }
]
```

Stage and commit the batch:

```sh
spl add --branch main --batch implementation-batch.json
spl status --branch main
spl commit --branch main --author "Engineer <eng@example.com>" --message "Record ephemeral technical spec for outbox relay worker"
```

---

## 5. Querying Technical Implementation Knowledge

```sh
# Discover active technical specs and plans
spl filter --branch main --label TechnicalSpec

# Find all ephemeral implementation nodes
spl filter --branch main --label Ephemeral

# Inspect full context of an implementation spec (upstream requirements, ADRs, standards)
spl context --branch main --query "outbox relay worker" --direction both --max-depth 2

# Resolve a specific technical specification node
spl resolve --branch main --node spec-tx-outbox-relay-worker
```
