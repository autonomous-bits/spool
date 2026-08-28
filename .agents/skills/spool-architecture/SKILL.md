---
name: spool-architecture
description: Author and query architecture knowledge in Spool. Defines labels, atomic node structures, and relationship edges for architecture decisions (ADRs), system boundaries, components, data contracts, and quality attributes.
---

# Spool Architecture Knowledge

Use this skill when capturing, refining, or querying **architecture decisions, topologies, technical invariants, and product-architecture mappings** in a Spool graph.

> [!IMPORTANT]
> **Context Scope & Domain Invariants**
> 1. **Scoped Context (Architecture & Product Only)**: Architecture work cares exclusively about **`Architecture`** and **`Product`** labels. When evaluating context, tracing dependencies, or running queries, ignore unrelated engineering standards, coding conventions, or lint rules.
> 2. **Knowledge Graph, Not Task Tracking**: Architecture models structural decisions, data contracts, system boundaries, and trade-offs. It is **never** used for project tasks (e.g. "Migrate database", "Write service boilerplate", "Deploy Redis"). Every node must be an atomic statement of architectural fact or decision.

---

## 1. Domain Taxonomy & Allowed Labels

Architecture queries and authoring operate within two scoped label sets:

### Architecture Labels (Primary Scope)
| Label | Purpose | Example Title |
| --- | --- | --- |
| `Architecture` | Top-level domain tag applied to all architecture knowledge nodes | *(Used with sub-labels)* |
| `Decision` / `ADR` | A distinct architectural decision record | `Order service publishes domain events via transactional outbox pattern to Apache Kafka` |
| `Component` | An atomic service, library, subsystem, or bounded context | `Billing Engine microservice responsible for invoice generation and payment gateway dispatch` |
| `Boundary` | Domain boundary, isolation tier, or network zone | `PCI-DSS Compliant Cardholder Data Environment (CDE)` |
| `Pattern` | A recognized design or integration pattern | `CQRS with separate read projections updated asynchronously via event streams` |
| `DataContract` / `Schema` | An immutable entity contract, event payload schema, or state machine | `OrderPlacedEvent protobuf message contract defining required order fields` |
| `QualityAttribute` | Latency, throughput, availability, or durability target (SLO/SLA) | `User authentication endpoint response time must be under 50ms at p99` |
| `Tradeoff` | An explicitly accepted downside, cost, or complexity | `Eventual consistency on order search projections introduces up to 5 seconds of indexing lag` |
| `Assumption` | An underlying infrastructure or platform premise | `Kubernetes cluster nodes span 3 availability zones with sub-2ms inter-zone latency` |

### Relevant Product Labels (Referenced Scope)
| Label | Purpose in Architecture Context |
| --- | --- |
| `Product`, `Requirement`, `Capability`, `Constraint`, `Problem` | Product requirements and business constraints that architectural decisions satisfy (`SATISFIES`) or are bound by (`CONSTRAINS`). |

> [!NOTE]
> Ignore `Standard`, `Convention`, `SecurityPolicy`, `TestingStandard`, and other engineering standard labels during architectural design and query analysis.

---

## 2. Standard Edge Relationships

Link atomic architecture ideas to architectural components and product requirements:

| Edge Type | Source Node | Target Node | Meaning |
| --- | --- | --- | --- |
| `SATISFIES` | `Decision` / `Component` | `Requirement` *(Product)* | The architectural choice fulfills the product requirement. |
| `COMMUNICATES_WITH` | `Component` | `Component` | A direct RPC, HTTP, or async communication path between components. |
| `DEFINES` | `Component` | `DataContract` | The component owns and produces the data contract or schema. |
| `INCURS` | `Decision` | `Tradeoff` | The decision explicitly accepts this drawback or cost. |
| `CONSTRAINS` | `QualityAttribute` / `Boundary` / `Constraint` *(Product)* | `Component` / `Decision` | Non-functional target or business boundary constrains the architecture. |
| `DEPENDS_ON` | `Component` | `Component` / Infrastructure | Structural runtime or deployment dependency. |
| `SUPERSEDES` | `Decision` | `Decision` | A newer architectural decision replaces an obsolete one. |
| `CONFLICTS_WITH` | `Decision` | `Decision` | Conflicting architectural approaches or unaligned boundaries. |

---

## 3. Node Content & Title Guidelines

### Atomic, Self-Contained Titles
Titles must express concrete decisions or structural facts with sufficient context:

- **Good**: `Account ledger records balances using immutable append-only entries rather than in-place updates`
- **Bad**: `Database design` *(Vague, uninformative)*
- **Bad**: `Refactor database schema to append-only` *(Task-oriented)*
- **Bad**: `Kafka and PostgreSQL setup for orders and billing` *(Combines multiple architectural concerns)*

### Anti-Patterns to Avoid
1. **Task Language**: Never author nodes like "Setup Redis cluster" or "Install Envoy". State the decision: `Distributed cache layer utilizes Redis with cluster-mode replication`.
2. **Coupled Tradeoffs**: Do not bury drawbacks in markdown paragraphs. Author a `Tradeoff` node and connect it to the `Decision` with an `INCURS` edge so agents can analyze trade-off graphs.
3. **Missing Product Links**: Whenever an architectural decision is driven by a product requirement, connect them via `SATISFIES`.

---

## 4. Batch Authoring Example

Create an `spl add` batch JSON file:

```json
[
  {
    "action": "add",
    "entity": "node",
    "id": "adr-order-outbox-kafka",
    "title": "Order service persists state to PostgreSQL and publishes domain events using a transactional outbox",
    "labels": ["Architecture", "Decision"],
    "properties": {
      "status": {"kind": "string", "string": "accepted"},
      "database": {"kind": "string", "string": "PostgreSQL 16"},
      "broker": {"kind": "string", "string": "Apache Kafka 3.6"}
    }
  },
  {
    "action": "add",
    "entity": "node",
    "id": "tradeoff-outbox-relay-latency",
    "title": "Transactional outbox relay introduces 100ms-500ms delay between DB commit and Kafka publication",
    "labels": ["Architecture", "Tradeoff"],
    "properties": {
      "impact": {"kind": "string", "string": "acceptable for asynchronous downstream notifications"}
    }
  },
  {
    "action": "add",
    "entity": "edge",
    "id": "edge-order-outbox-incurs-relay-latency",
    "source": "adr-order-outbox-kafka",
    "target": "tradeoff-outbox-relay-latency",
    "type": "INCURS"
  },
  {
    "action": "add",
    "entity": "edge",
    "id": "edge-order-outbox-satisfies-order-req",
    "source": "adr-order-outbox-kafka",
    "target": "req-deferred-billing-address",
    "type": "SATISFIES"
  }
]
```

Stage and commit the batch:

```sh
spl add --branch main --batch arch-batch.json
spl status --branch main
spl commit --branch main --author "Architect <arch@example.com>" --message "Record transactional outbox architecture decision"
```

---

## 5. Querying Architecture Knowledge

When querying architecture context, limit scope to **Architecture** and **Product** nodes:

```sh
# Search architecture decisions regarding messaging or storage
spl search --branch main --query "outbox Kafka"

# List all architecture decisions
spl filter --branch main --label Decision

# Resolve a specific node by its exact ID
spl resolve --branch main --node req-deferred-billing-address

# Inspect bounded context around an architectural decision or requirement
spl context --branch main --query "deferred billing" --direction both --max-depth 2

# Inspect all tradeoffs incurred by system decisions
spl filter --branch main --label Tradeoff

# Filter for all components in the system architecture
spl filter --branch main --label Component
```
