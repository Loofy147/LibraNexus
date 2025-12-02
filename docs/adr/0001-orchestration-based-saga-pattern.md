# ADR 001: Orchestration-Based Saga Pattern for Distributed Transactions

## Status

Proposed

## Context

The LibraNexus system is designed as a set of distributed microservices. Certain business processes, such as a member checking out a book, require coordinating updates across multiple services (e.g., `Circulation` and `Catalog`). In a distributed system, ensuring data consistency across these services is a challenge, as traditional ACID transactions are not viable.

The Saga pattern is a common solution for managing distributed transactions. It breaks a global transaction into a series of local transactions that can be interleaved. If any local transaction fails, the saga executes a series of compensating transactions to undo the preceding transactions.

There are two primary ways to coordinate sagas:
1.  **Choreography:** Each service publishes events that trigger the next local transaction in the saga. This is a decentralized approach where services are loosely coupled.
2.  **Orchestration:** A central orchestrator (or coordinator) is responsible for telling the saga participants what to do and when. This is a centralized approach that provides a clear overview of the entire transaction flow.

## Decision

We will use an **orchestration-based Saga pattern** for managing distributed transactions in LibraNexus. The `Circulation` service will act as the orchestrator for the checkout and return processes.

The `CheckoutItem` saga will be orchestrated as follows:
1.  The `Circulation` service receives a checkout request.
2.  It calls the `Membership` service to validate the member's status.
3.  It calls the `Catalog` service to check item availability and decrement the available count.
4.  It creates a checkout record in its own database.

If any step fails, the `Circulation` service will execute compensating transactions. For example, if creating the checkout record fails, it will call the `Catalog` service to increment the item's available count.

## Consequences

*   **Pros:**
    *   **Centralized Logic:** The entire transaction flow is defined in one place, making it easier to understand, debug, and maintain.
    *   **Explicit State Management:** The orchestrator explicitly manages the state of the saga, which simplifies error handling and recovery.
    *   **Reduced Service Coupling:** Participant services do not need to be aware of the overall saga, reducing coupling between services.
*   **Cons:**
    *   **Single Point of Failure:** The orchestrator can become a single point of failure. This will be mitigated by making the `Circulation` service highly available.
    *   **Potential for Bottlenecks:** The orchestrator can become a bottleneck if it manages a large number of sagas. This will be monitored, and the `Circulation` service will be scaled as needed.