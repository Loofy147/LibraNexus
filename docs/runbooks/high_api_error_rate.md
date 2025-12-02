# Runbook: High API Error Rate

## Summary

This runbook provides a step-by-step guide for responding to a high API error rate alert from the LibraNexus system.

## Alerting

*   **Alert:** `HighErrorRate`
*   **Threshold:** > 5% error rate over a 5-minute period.
*   **Severity:** Critical

## Initial Triage

1.  **Acknowledge the alert:** Acknowledge the alert in the monitoring system to inform the team that you are investigating.
2.  **Check the dashboards:** Open the Grafana dashboards for the LibraNexus system and look for any anomalies.
    *   Is there a spike in traffic?
    *   Are any of the services showing high CPU or memory usage?
    *   Are there any errors in the logs?
3.  **Identify the affected services:** Use the dashboards and logs to identify which services are generating the errors.
    *   Are the errors coming from a single service or multiple services?
    *   Are the errors related to a specific endpoint?

## Escalation

*   If the issue is not resolved within 15 minutes, escalate to the on-call engineer for the affected service.
*   If the issue is affecting multiple services, escalate to the on-call SRE.

## Remediation

### Common Causes and Solutions

*   **Bad deployment:** If the error rate started after a recent deployment, roll back the deployment.
*   **Database issue:** If the errors are related to the database, check the database dashboards for high CPU usage, connection pool exhaustion, or slow queries.
    *   If the database is overloaded, consider scaling it up.
    *   If there are slow queries, work with the development team to optimize them.
*   **Upstream dependency issue:** If the errors are related to an upstream dependency (e.g., a third-party API), check the status page for the dependency and contact their support team if necessary.
*   **Spike in traffic:** If there is a sudden spike in traffic, consider scaling up the affected services.

### Specific Scenarios

*   **Catalog service is failing:**
    *   Check the Meilisearch dashboard for any issues. If Meilisearch is down, the service should be falling back to the database. If the fallback is not working, investigate the circuit breaker configuration.
*   **Circulation service is failing:**
    *   Check the logs for errors related to the Saga pattern. If there are compensation failures, investigate the state of the saga.
*   **Membership service is failing:**
    *   Check the logs for errors related to password hashing or authentication.

## Post-Mortem

*   Once the issue is resolved, create a post-mortem to document the root cause of the issue and the steps taken to resolve it.
*   Identify any action items that can be taken to prevent the issue from happening again.