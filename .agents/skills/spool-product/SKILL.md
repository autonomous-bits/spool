---
name: spool-product
description: Author and query product knowledge in Spool. Defines labels, atomic node structures, and relationship edges for problems, personas, outcomes, capabilities, requirements, and constraints.
---

# Spool Product Knowledge

Use this skill when capturing, refining, or querying **product-related knowledge** in a Spool graph.

> [!IMPORTANT]
> **Core Product Invariants**
> 1. **Knowledge Graph, Not Task Tracking**: Spool captures atomic ideas, invariant business rules, and user needs. It is **never** used for transient tasks (e.g. "Implement feature X", "Write tests for Y", "Fix bug Z").
> 2. **Strict Label Isolation**: Product work must **only** use product-related labels. Never apply architecture, engineering, or technical labels to product nodes.
> 3. **Strictly Business-Oriented (Zero Technical Jargon)**: Product entries must be written strictly from the perspective of user value, business rules, customer workflows, and domain outcomes. Never use technical implementation details (e.g. no database names, protocols, API endpoints, JSON/REST/Kafka specs, frameworks, or code concepts).

---

## 1. Domain Taxonomy & Allowed Labels

Product nodes must strictly draw from the following product-only labels:

| Label | Purpose | Example Title |
| --- | --- | --- |
| `Product` | Mandatory top-level domain tag applied to all product knowledge nodes | *(Used with sub-labels)* |
| `Problem` | A validated customer friction, operational bottleneck, or unmet market need | `Customers abandon checkout when billing address is required before viewing shipping costs` |
| `Persona` | A specific customer segment, user archetype, or business role | `Enterprise Compliance Auditor verifying quarterly access controls` |
| `Outcome` / `Goal` | A measurable business or customer outcome (not a technical solution) | `Reduce checkout abandonment rate by 15% across mobile shoppers` |
| `Capability` | A high-level business ability or functional system capability | `Customer self-service account recovery and security verification` |
| `Requirement` | An atomic, unambiguous functional or business rule | `Password reset links expire exactly 30 minutes after issuance` |
| `Constraint` | A legal, statutory, regulatory, or business boundary | `Customer financial records must be retained for at least 7 years per financial regulations` |
| `Assumption` | An explicit product hypothesis or unvalidated market belief | `Small business owners prefer automated monthly invoicing over on-demand billing` |
| `Metric` | A quantitative business KPI or success metric definition | `Checkout conversion rate defined as completed purchases divided by started checkouts` |

> [!WARNING]
> **Forbidden in Product Work**
> - **Labels**: `Architecture`, `Decision`, `ADR`, `Component`, `Standard`, `APIStandard`, `SecurityPolicy`, `DataContract`, `Pattern`, etc.
> - **Technical Concepts**: Do not reference databases (SQL, Redis, Postgres), protocols/formats (REST, gRPC, JSON, WebSocket), architecture topology (microservices, Kafka, queues), or UI code widgets.

---

## 2. Standard Product Edge Relationships

Link atomic product ideas using explicit, semantic product relationships:

| Edge Type | Source Node | Target Node | Meaning |
| --- | --- | --- | --- |
| `ADDRESSES` | `Requirement` / `Capability` | `Problem` | The business rule or capability mitigates the customer pain point. |
| `ENABLES` | `Capability` | `Outcome` | The capability enables achieving the business outcome. |
| `TARGETS` | `Capability` / `Requirement` | `Persona` | The product feature is designed for this user role or persona. |
| `CONSTRAINS` | `Constraint` | `Requirement` / `Capability` | The legal/business constraint bounds the requirement. |
| `REFINES` | `Requirement` | `Capability` | The requirement provides detailed criteria for the capability. |
| `MEASURES` | `Metric` | `Outcome` | The metric quantifies progress toward the outcome. |
| `DEPENDS_ON` | `Requirement` | `Requirement` | The requirement logically requires another product concept. |
| `SUPERSEDES` | `Requirement` | `Requirement` | A newer requirement replaces an obsolete business rule. |
| `CONFLICTS_WITH` | `Requirement` | `Requirement` | Two business requirements contradict each other. |

---

## 3. Business-Oriented Content Guidelines

### Business-Oriented vs. Technical Phrasing

| Business-Oriented (Correct) | Technical Implementation (Forbidden in Product) | Why Technical Fails |
| --- | --- | --- |
| `Audit reports must provide exportable transaction logs for compliance verification` | `Stream audit logs via NDJSON over WebSocket with Redis buffering` | Specifies technical transmission format and infrastructure rather than user need. |
| `Users must verify their identity using a second factor before changing billing details` | `Authenticate via TOTP RFC 6238 endpoint with Redis session store` | Specifies specific cryptographic algorithms and storage engines instead of business policy. |
| `Customers receive an itemized receipt by email immediately after purchase completion` | `Publish OrderCompletedEvent to Kafka to trigger SendGrid API call` | Describes event brokers and third-party vendor APIs instead of the user experience. |
| `Subscription renewals occur automatically on the monthly anniversary of sign-up` | `Run cron worker to execute batch SQL UPDATE queries on customer_subscriptions table` | Describes batch cron scripts and database tables rather than product behavior. |

### Anti-Patterns to Avoid
1. **Technical Specifications**: Keep technical decisions in `spool-architecture` and technical rules in `spool-engineering-standards`. Product nodes must only state *what* value or rule exists, never *how* engineers will build it.
2. **Task Language**: Do not author "Implement checkout", "Create ticket", or "Design UI mockup". State the business rule: `Checkout requires order review before payment submission`.
3. **Compound Nodes**: Never combine a problem and a solution into one node. Split into a `Problem` node and a `Requirement` node linked by `ADDRESSES`.

---

## 4. Batch Authoring Example

Create a purely business-oriented `spl add` batch JSON file:

```json
[
  {
    "action": "add",
    "entity": "node",
    "id": "prob-checkout-address-friction",
    "title": "Customers abandon checkout when billing address is required before seeing shipping options",
    "labels": ["Product", "Problem"],
    "properties": {
      "severity": {"kind": "string", "string": "high"},
      "impact": {"kind": "string", "string": "12% cart abandonment at step 2"}
    }
  },
  {
    "action": "add",
    "entity": "node",
    "id": "req-deferred-billing-address",
    "title": "Checkout collects billing address only during final payment method submission",
    "labels": ["Product", "Requirement"],
    "properties": {
      "businessValue": {"kind": "string", "string": "Reduces upfront form friction for mobile shoppers"},
      "priority": {"kind": "string", "string": "high"}
    }
  },
  {
    "action": "add",
    "entity": "edge",
    "id": "edge-deferred-billing-addresses-friction",
    "source": "req-deferred-billing-address",
    "target": "prob-checkout-address-friction",
    "type": "ADDRESSES"
  }
]
```

Stage and commit the batch:

```sh
spl add --branch main --batch product-batch.json
spl status --branch main
spl commit --branch main --author "Product Manager <pm@example.com>" --message "Record deferred billing address requirement"
```

---

## 5. Querying Product Knowledge

Discover and traverse product knowledge using `spl`:

```sh
# Search for product requirements related to billing
spl search --branch main --query "billing address"

# Filter for all open product problems
spl filter --branch main --label Problem

# Filter for all product personas
spl filter --branch main --label Persona

# Resolve a specific requirement by its exact ID
spl resolve --branch main --node req-deferred-billing-address

# Inspect context around a product domain concept
spl context --branch main --query "billing address" --direction both --max-depth 2
```
