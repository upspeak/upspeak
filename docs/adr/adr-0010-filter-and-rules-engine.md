---
title: "ADR-0010: Reusable filter engine and hop-bounded rules engine"
status: "Accepted"
date: "2026-06-05"
authors: "Kaustav Das Modak, Claude"
tags: ["filters", "rules", "automation"]
supersedes: ""
superseded_by: ""
---

# ADR-0010: Reusable filter engine and hop-bounded rules engine

## Status

**Accepted**

## Context

Several features need to express "does this thing match these conditions?": connector
sources and sinks filter what they collect or publish, and rules decide whether to fire
on an event. Re-implementing condition evaluation in each place would drift in operator
semantics and field-resolution behaviour.

Separately, an automation engine that reacts to events by performing actions — which can
themselves emit events — risks **infinite cascades** (a rule whose action triggers the
same rule). Automation needed to be useful without being able to run away.

## Decision

Build **one reusable filter engine** and a **rules engine that reuses it**, with an
explicit cascade bound.

- **Filter engine** (`filter/engine.go`): condition sets with **15 operators**
  (`eq`, `neq`, `contains`, `not_contains`, `starts_with`, `ends_with`, `in`, `not_in`,
  `gt`, `lt`, `gte`, `lte`, `exists`, `not_exists`, `matches`), **dot-path** field
  resolution into event/entity payloads, and **AND/OR** combination modes. Operators are
  defined once in `core` (`ConditionOp`). Filters are first-class, CRUD-managed entities
  reused by sources, sinks, and rules.
- **Rules engine** (`rules/engine.go`): a durable `REPO_EVENTS` consumer
  ([[adr-0009-global-repo-events-stream]]) implementing event-condition-action triggers.
  The condition step is evaluated by the filter engine; actions include enrich, relate,
  annotate, collect, publish, and webhook.
- **Hop-bounded cascades**: `core.Event` carries a `Hops` counter. The engine **drops**
  any event whose `Hops >= maxRuleHops` (`maxRuleHops = 5`) and **stamps `Hops+1`** on
  events it emits as reactions. Events emitted as rule reactions must propagate `Hops`.

## Consequences

### Positive

- **POS-001**: One condition-evaluation implementation, so operator and dot-path
  semantics are identical for sources, sinks, and rules.
- **POS-002**: Filters are reusable, named entities — define once, reference from many
  places — with referential-integrity checks on delete.
- **POS-003**: The hop limit makes automation safe by construction: a misconfigured
  rule loop terminates after at most `maxRuleHops` reactions instead of running forever.
- **POS-004**: The rules engine inherits durable, at-least-once, crash-safe delivery
  from `REPO_EVENTS`.

### Negative

- **NEG-001**: The hop limit is a blunt global bound — a legitimately deep but
  non-cyclic reaction chain longer than `maxRuleHops` is also cut off.
- **NEG-002**: Dot-path resolution depends on stable event payload shapes; renaming a
  payload field can silently break a filter that referenced it.
- **NEG-003**: Reaction events must remember to carry `Hops` forward; a future event
  producer that forgets to propagate it would weaken the cascade bound.

## Alternatives Considered

### Per-feature ad-hoc condition checks

- **ALT-001**: **Description**: Let each module implement its own matching logic.
- **ALT-002**: **Rejection Reason**: Guarantees semantic drift between sources, sinks,
  and rules, and triples the test surface.

### An embedded scripting/expression language for rules

- **ALT-003**: **Description**: Allow arbitrary user expressions/scripts as conditions.
- **ALT-004**: **Rejection Reason**: Far larger attack and complexity surface than a
  fixed operator set; the 15 operators plus AND/OR cover the needed cases.

### Cycle detection by tracking event lineage

- **ALT-005**: **Description**: Detect loops by recording and inspecting causal event
  chains.
- **ALT-006**: **Rejection Reason**: More state and complexity than warranted; a simple
  hop counter bounds cascades cheaply and predictably (the connector path additionally
  has its own cycle detection where graph cycles matter).

## Implementation Notes

- **IMP-001**: Operators in `core/shared_types.go` (`ConditionOp`); evaluation in
  `filter/engine.go` (`applyOperator`). Filter CRUD in `filter/filter.go`.
- **IMP-002**: Rules engine in `rules/engine.go`; `maxRuleHops` is a package constant
  (currently 5); `core.Event.Hops` carries the count.
- **IMP-003**: Any new producer of rule-reaction events must propagate `Hops` so the
  cascade bound holds; consider making the hop limit configurable if real workloads need
  deeper chains.

## References

- **REF-001**: [[adr-0009-global-repo-events-stream]] — the stream the rules engine
  consumes.
- **REF-002**: [[adr-0008-hybrid-sync-write-async-events]] — the events rules react to.
- **REF-003**: [[adr-0006-http-api-conventions]] — filters and rules are CRUD entities
  on the API (rules use the typed-self-contained-URL idiom).
