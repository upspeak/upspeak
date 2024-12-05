# Upspeak Core Package

The `core` package defines the foundational abstractions and types central to Upspeak. It encapsulates Upspeak’s core domain, applying Domain-Driven Design (DDD) principles pragmatically[^1] to balance conceptual clarity, scalability, and flexibility. This README outlines the package’s design philosophy, architecture, and key domain concepts.

---

## Core Design Philosophy

The `core` package is designed to address the primary goal of Upspeak: enabling robust, scalable, and extensible knowledge graphs. Its foundation is built on **domain-first thinking**, ensuring core abstractions like `Node`, `Edge`, `Thread`, and `Annotation` align with real-world concepts.

Key design principles include:

1. **Separation of Concerns:** Domain logic is independent of infrastructure concerns such as storage and messaging, simplifying testing, extensibility, and integration.
2. **Interoperability:** Core abstractions use standardized formats, such as JSON for data and the Web Annotations API for metadata, enabling seamless integration with external systems.
3. **Flexibility with Simplicity:** While influenced by DDD, the design avoids unnecessary complexity, adapting concepts pragmatically to Upspeak’s evolving requirements.

These principles establish a modular and extensible foundation, allowing the system to address immediate needs while remaining adaptable for future requirements.

---

## Architectural Approach

The architecture emphasizes composability, separation of concerns, and event-driven design, with decoupled components operating in clearly defined contexts.

### Benefits

1. **Adaptability:** The design supports a wide range of use cases, including hierarchical structures, annotations, and event-driven systems.
2. **Extensible Domain Model:** The abstractions are generic enough to model different data types (e.g., Discourse topics, Matrix events) while preserving the integrity of the underlying data structure.
3. **Testability:** Decoupled domain logic can be independently verified, simplifying testing and debugging.
4. **Interoperability:** Clear boundaries between the core and external systems (e.g., storage, messaging) ensure easier integrations and updates.

### Tradeoffs

1. **Generic Abstractions vs Specificity:**
   - `Node` and `Edge` metadata use `json.RawMessage` for flexibility but lack compile-time type safety, necessitating additional validation and transformation logic. There are some [known performance penalties](https://github.com/golang/go/issues/33422) that may improve with the future Go versions.
   - While generics in Go could improve type safety, [prior attempts](https://github.com/upspeak/upspeak/blob/aa1d6cce16895aff1c2873f4175d2f2fcbed1c9d/core/node.go#L63) were infeasible across the full domain model. Revisiting generics may streamline future implementations.

2. **Hierarchical Structures:**  
   - Threads allow nested Nodes and sub-Threads for complex, branching conversations but increase the complexity of queries, indexing, and data consistency.

3. **Event-Driven Design:**  
   - Decoupling operations from side effects enhances scalability but requires rigorous event orchestration, idempotency, and robust failure handling. Hence, the initial choice to use [NATS](https://nats.io).

4. **Repository-Scoped Archives:**  
   - Scoping `Archive` to a `Repository` ensures isolation and backend customizability but complicates cross-Repository queries and data sharing. This can be mitigated by creating cross-repo querying capabilities in the future.

5. **Annotation Modelling:**  
   - Treating Annotations as specialized Nodes aligns with the Web Annotations API and ensures consistency, but may add overhead in annotation-heavy scenarios.

6. **Decoupled Domain Logic:**  
   - Isolating domain logic improves maintainability but shifts complexity to integration layers requiring well-defined adapters.

---

## Core Concepts

### Node

`Node` represents a discrete unit of knowledge and supports arbitrary nested JSON structures for metadata and body. This abstraction makes it suitable for modelling diverse entities like comments, topics, or messages.

### Edge

`Edge` models relationships between Nodes, forming the backbone of the knowledge graph. It defines source and target Nodes, with an optional `Type` for contextual relationships (e.g., replies, dependencies).

### Thread

A `Thread` is a composite structure representing hierarchical or nested discussions. Threads enable organizing related Nodes into branches or forks, akin to GitHub issues with threaded comments.

A Thread consists of one or more Nodes, may include sub-Threads, and eventually[^2] support branching, forking, and merging of threads. Ideal for creating multi-level discussions, contextual responses, and knowledge tree visualisations.

### Annotation

Annotations extend Nodes and Edges with additional metadata, leveraging the Web Annotations API. This design standardizes the representation of comments, tags, or other contextual information.

### Event

`Event` encapsulates domain operations and their results, enabling event-driven workflows. Events specify an `Event.Type` and a corresponding `Payload`, decoupling operations from side effects and supporting external integrations (e.g., NATS).

### Archive

`Archive` is the scoped data store for a `Repository`, managing Nodes, Edges, Threads, and Annotations. It abstracts backend-specific implementations, ensuring domain logic remains backend-agnostic.

### Repository

A `Repository` represents a collection of Nodes, Edges, Threads, and Annotations scoped to a specific context.

A Repository accepts events denoting operations via an input stream, performs corresponding operations against its associated Archive and publishes resulting events to an output stream.

---

This package is a work in progress, and contributions are welcome to refine the abstractions and build a robust foundation for Upspeak.

[^1]: I know DDD practitioners warn against partial DDD implementations, but sometimes it is better to adapt better practices and incrementally evolve to best practices, especially when working alone or in a small team. This is one such scenario.
[^2]: This is already possible by creating Edges, but let's wait for specific implementations to complete before we can claim it.
