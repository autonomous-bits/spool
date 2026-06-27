<!--
SYNC IMPACT REPORT
==================
Version change: (unratified) -> 1.0.0
Bump rationale: Initial ratification of Spool's engineering constitution.

Principles defined:
  I.   Monorepo Discipline
  II.  Containerized API Development and Testing
  III. Vertical Slice Architecture
  IV.  Rich Domain Models
  V.   Test-Driven Development

Added sections:
  - Development Workflow and Quality Gates
  - Governance

Follow-up TODOs: none.
-->

# Spool Constitution

Spool turns stakeholder chat into approved implementation context. The system is built as a
TypeScript/NestJS monorepo with a store API, MCP server, shared tooling, and product and
architecture documentation. This constitution defines the engineering rules that guide every
change to the repository.

## Core Principles

### I. Monorepo Discipline

Spool is a monorepo, and changes MUST preserve clear ownership boundaries between workspaces.

- Application code lives under `apps/`. The store API lives in `apps/store`; the MCP server
  lives in `apps/mcp`.
- Shared scripts, code generation, and CLI helpers live under `tools/`. Application code MUST
  NOT import from `tools/`.
- Shared environment templates and runtime configuration live under `config/`; secrets belong
  in local environment files, never in source control.
- Cross-workspace changes MUST be coordinated so build, typecheck, tests, documentation, and
  runtime configuration remain consistent across the repository.

**Rationale:** A monorepo lets Spool evolve store, MCP, tooling, and documentation together, but
only if workspace boundaries stay explicit and enforceable.

### II. Containerized API Development and Testing

APIs such as the store MUST be developed and tested through containers when running local
runtime dependencies.

- The store API runs locally with Docker Compose. Do not run the store directly on the host for
  local development unless the task explicitly requires it.
- Runtime dependencies such as Postgres MUST be supplied through the repository's container
  configuration so local, CI, and agent environments converge.
- Container configuration is part of the product surface. Changes that affect API runtime
  behavior MUST update Compose, environment templates, and related documentation together.
- Tests SHOULD use the smallest deterministic setup that proves the behavior, but API-level
  behavior depending on runtime services MUST remain compatible with the containerized setup.

**Rationale:** Containerized APIs keep development reproducible, reduce host-specific drift, and
make failures easier to reproduce across contributors and agents.

### III. Vertical Slice Architecture

Features MUST be organized around vertical slices of behavior rather than horizontal technical
layers alone.

- A slice owns its request handling, validation, application logic, domain behavior,
  persistence access, and tests for a coherent user or agent capability.
- New behavior SHOULD be added inside the slice that owns the capability. Create a new slice
  only when the boundary represents a distinct workflow or domain concept.
- Cross-cutting infrastructure is allowed, but it MUST support slices rather than pull business
  behavior into generic service layers.
- Slices MUST expose clear contracts and avoid hidden coupling through shared mutable state,
  implicit globals, or unrelated module imports.

**Rationale:** Vertical slices keep stakeholder workflows, agent interactions, and persistence
rules understandable as complete units of behavior.

### IV. Rich Domain Models

Spool MUST model domain concepts with concrete value types and entities, not anemic data bags.

- Core concepts such as tenants, idea chunks, lifecycle states, relationships, graph edges, and
  generated projections SHOULD be represented by named value types or entities with behavior and
  invariants.
- Domain rules MUST live with the model that owns them whenever practical. Avoid scattering
  validation and state transitions across controllers, DTO mappers, or generic services.
- Primitive obsession is discouraged. Prefer explicit identifiers, lifecycle states, typed
  relationship names, and domain methods over unvalidated strings and loosely shaped objects.
- Persistence schemas and DTOs are boundaries, not the domain model. Mapping code MUST preserve
  domain invariants rather than bypass them.

**Rationale:** The source of truth is a tenant-scoped graph of idea chunks and relationships.
Rich models make lifecycle and graph invariants visible, testable, and hard to misuse.

### V. Test-Driven Development

Spool follows test-driven development using the blue, green, refactor loop.

- **Blue:** describe the desired behavior with a focused failing test or executable
  specification before implementing the behavior.
- **Green:** implement the smallest correct change that makes the focused test pass without
  weakening existing guarantees.
- **Refactor:** improve structure, names, and boundaries while keeping the test suite green.
- Behavioral changes MUST include automated tests at the right level: unit tests for domain
  rules, integration tests for slice wiring, and API or MCP tests for externally visible
  behavior.
- Tests MUST be deterministic and must not depend on uncontrolled network access, wall-clock
  timing, or host-specific state.

**Rationale:** TDD keeps Spool aligned with approved behavior, protects the graph lifecycle, and
prevents implementation details from outrunning stakeholder intent.

## Development Workflow and Quality Gates

- Repository checks are `pnpm build`, `pnpm typecheck`, and `pnpm test`; changes MUST keep these
  gates passing.
- Prefer targeted checks while developing, then run the relevant workspace or repository gate
  before considering code complete.
- Documentation MUST change with behavior when the change affects product workflows,
  architecture constraints, runtime setup, or contributor expectations.
- Security-sensitive code, request handling, validation, SQL access, secrets, shutdown paths,
  and logging MUST follow the repository's established hardening and structured logging
  patterns.
- Pull requests SHOULD stay focused on one capability or correction and avoid unrelated cleanup.

## Governance

This constitution supersedes ad-hoc convention where they conflict. Exceptions require explicit
approval from maintainers and MUST document the tradeoff, affected principles, and a follow-up
plan when the exception is temporary.

Amendments require:

1. A documented proposal describing the changed principle or governance rule.
2. Maintainer approval.
3. Updates to affected documentation, templates, tests, or workflow gates.
4. A constitution version bump using semantic versioning.

**Version:** 1.0.0  
**Ratified:** 2026-06-27  
**Last Amended:** 2026-06-27
