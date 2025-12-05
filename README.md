# LibraNexus: A Reference Implementation

> **Note:** This is an educational project demonstrating distributed systems
> patterns, not a production library system. For production use, consider
> Koha (open-source) or SirsiDynix (commercial).

## What This Demonstrates
- Event sourcing with PostgreSQL
- Saga pattern for distributed transactions
- Chaos engineering with automated fault injection
- OpenTelemetry distributed tracing
- Kubernetes deployment with Istio service mesh

## Architecture Trade-offs
This project intentionally over-engineers a library system to showcase
advanced patterns. In production, most of these patterns would be overkill:
- Event sourcing → Use audit log tables
- Saga pattern → Use database transactions
- Kubernetes → Use AWS ECS Fargate

## Learning Resources
- [ADR-001: Why Orchestration Over Choreography](docs/adr/0001...)
- [Chaos Experiments: Results & Analysis](docs/chaos/...)
- [Cost Analysis: When Complexity Isn't Worth It](docs/costs.md)