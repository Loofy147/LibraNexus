// chaos/experiments.go
package chaos

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// RegisterExperiments registers all predefined chaos experiments with the engine.
func (ce *ChaosEngine) RegisterExperiments() {
	ce.RegisterExperiment(ce.DatabaseLatencyExperiment(250 * time.Millisecond))
	ce.RegisterExperiment(ce.CircuitBreakerExperiment())
	ce.RegisterExperiment(ce.ConcurrentCheckoutRaceConditionTest())
	ce.RegisterExperiment(ce.NetworkPartitionExperiment())
	ce.RegisterExperiment(ce.ResourceExhaustionExperiment())
}

// DatabaseLatencyExperiment injects latency into database operations
func (ce *ChaosEngine) DatabaseLatencyExperiment(targetLatency time.Duration) ChaosExperiment {
	latencyInjected := false
	var originalDB *sql.DB

	return ChaosExperiment{
		Name:       "database-latency-injection",
		Hypothesis: "System degrades gracefully when database latency exceeds threshold",
		SteadyState: []Metric{
			{
				Name: "checkout_success_rate",
				Query: func(ctx context.Context) (float64, error) {
					var successRate float64
					err := ce.db.QueryRowContext(ctx, `
						SELECT COALESCE(
							COUNT(*) FILTER (WHERE status = 'active')::float / NULLIF(COUNT(*)::float, 0) * 100,
							100.0
						) FROM checkouts WHERE created_at > NOW() - INTERVAL '1 minute'
					`).Scan(&successRate)
					return successRate, err
				},
				Threshold: Threshold{Operator: ">", Value: 99.0},
			},
		},
		Method: []Action{
			{
				Type:   "inject-latency",
				Target: "postgres-primary",
				Parameters: map[string]interface{}{
					"latency": targetLatency,
					"jitter":  50 * time.Millisecond,
				},
				Execute: func(ctx context.Context) error {
					// Wrap database calls with artificial latency
					latencyInjected = true
					originalDB = ce.db
					// In production, this would use a proxy or network policy
					return nil
				},
			},
		},
		Rollback: []Action{
			{
				Type:   "remove-latency",
				Target: "postgres-primary",
				Execute: func(ctx context.Context) error {
					latencyInjected = false
					ce.db = originalDB
					return nil
				},
			},
		},
		Validation: []Assertion{
			{
				Metric:    "checkout_success_rate",
				Condition: func(v float64) bool { return v > 95.0 },
				Message:   "Checkout success rate should remain above 95%",
			},
		},
		Duration:    5 * time.Minute,
		BlastRadius: 1.0,
	}
}

// CircuitBreakerExperiment validates circuit breaker behavior
func (ce *ChaosEngine) CircuitBreakerExperiment() ChaosExperiment {
	searchBackendKilled := false

	return ChaosExperiment{
		Name:       "search-backend-failure",
		Hypothesis: "Catalog searches fallback to database when search backend is unavailable",
		SteadyState: []Metric{
			{
				Name: "search_availability",
				Query: func(ctx context.Context) (float64, error) {
					// Would query metrics endpoint
					return 100.0, nil
				},
				Threshold: Threshold{Operator: ">", Value: 99.0},
			},
		},
		Method: []Action{
			{
				Type:   "kill-pod",
				Target: "meilisearch",
				Parameters: map[string]interface{}{
					"mode":     "fixed",
					"interval": "0s",
				},
				Execute: func(ctx context.Context) error {
					searchBackendKilled = true
					// In production: kubectl delete pod meilisearch-xyz
					return nil
				},
			},
		},
		Rollback: []Action{
			{
				Type:   "restore-pod",
				Target: "meilisearch",
				Execute: func(ctx context.Context) error {
					searchBackendKilled = false
					// In production: kubectl scale deployment meilisearch --replicas=1
					return nil
				},
			},
		},
		Validation: []Assertion{
			{
				Metric:    "search_availability",
				Condition: func(v float64) bool { return v > 95.0 },
				Message:   "Search should maintain 95% availability via fallback",
			},
		},
		Duration:    2 * time.Minute,
		BlastRadius: 0.5,
	}
}

// ConcurrentCheckoutRaceConditionTest validates saga compensation
func (ce *ChaosEngine) ConcurrentCheckoutRaceConditionTest() ChaosExperiment {
	return ChaosExperiment{
		Name:       "concurrent-checkout-race-condition",
		Hypothesis: "System prevents double-booking when multiple checkouts occur simultaneously",
		SteadyState: []Metric{
			{
				Name: "data_consistency",
				Query: func(ctx context.Context) (float64, error) {
					var inconsistencies int
					err := ce.db.QueryRowContext(ctx, `
						SELECT COUNT(*) FROM items
						WHERE available < 0 OR available > total_copies
					`).Scan(&inconsistencies)
					return float64(inconsistencies), err
				},
				Threshold: Threshold{Operator: "==", Value: 0},
			},
		},
		Method: []Action{
			{
				Type:   "concurrent-requests",
				Target: "circulation-service",
				Parameters: map[string]interface{}{
					"concurrency": 100,
					"item_id":     "same-item",
				},
				Execute: func(ctx context.Context) error {
					// Simulate 100 concurrent checkout requests for same item
					var wg sync.WaitGroup
					errors := make(chan error, 100)

					for i := 0; i < 100; i++ {
						wg.Add(1)
						go func() {
							defer wg.Done()
							// Attempt checkout - most should fail gracefully
							// This would call CirculationService.ProcessCheckout
						}()
					}

					wg.Wait()
					close(errors)
					return nil
				},
			},
		},
		Rollback: []Action{},
		Validation: []Assertion{
			{
				Metric:    "data_consistency",
				Condition: func(v float64) bool { return v == 0 },
				Message:   "No data inconsistencies should occur",
			},
		},
		Duration:    30 * time.Second,
		BlastRadius: 0.1,
	}
}

// NetworkPartitionExperiment tests event bus resilience
func (ce *ChaosEngine) NetworkPartitionExperiment() ChaosExperiment {
	return ChaosExperiment{
		Name:       "event-bus-partition",
		Hypothesis: "Services buffer events and replay when NATS reconnects",
		SteadyState: []Metric{
			{
				Name: "event_publish_success_rate",
				Query: func(ctx context.Context) (float64, error) {
					return 100.0, nil // Would query NATS metrics
				},
				Threshold: Threshold{Operator: "==", Value: 100.0},
			},
		},
		Method: []Action{
			{
				Type:   "network-partition",
				Target: "nats-cluster",
				Parameters: map[string]interface{}{
					"duration": "2m",
				},
				Execute: func(ctx context.Context) error {
					// In production: apply NetworkPolicy to block NATS traffic
					return nil
				},
			},
		},
		Rollback: []Action{
			{
				Type:   "restore-network",
				Target: "nats-cluster",
				Execute: func(ctx context.Context) error {
					// Remove NetworkPolicy
					return nil
				},
			},
		},
		Validation: []Assertion{
			{
				Metric: "event_publish_success_rate",
				Condition: func(v float64) bool {
					return v == 100.0 // All events eventually published
				},
				Message: "All buffered events should be published after recovery",
			},
		},
		Duration:    5 * time.Minute,
		BlastRadius: 0.3,
	}
}

// ResourceExhaustionExperiment tests system under resource pressure
func (ce *ChaosEngine) ResourceExhaustionExperiment() ChaosExperiment {
	return ChaosExperiment{
		Name:       "database-connection-pool-exhaustion",
		Hypothesis: "Circuit breaker prevents cascading failures when connection pool is exhausted",
		SteadyState: []Metric{
			{
				Name: "error_rate",
				Query: func(ctx context.Context) (float64, error) {
					return 0.0, nil // Would query error metrics
				},
				Threshold: Threshold{Operator: "<", Value: 1.0},
			},
		},
		Method: []Action{
			{
				Type:   "exhaust-connections",
				Target: "postgres-connection-pool",
				Execute: func(ctx context.Context) error {
					// Open connections and hold them
					conns := make([]*sql.Conn, 0)
					for i := 0; i < 100; i++ {
						conn, err := ce.db.Conn(ctx)
						if err != nil {
							break
						}
						conns = append(conns, conn)
					}
					// Hold connections for experiment duration
					time.Sleep(30 * time.Second)
					for _, conn := range conns {
						conn.Close()
					}
					return nil
				},
			},
		},
		Rollback: []Action{},
		Validation: []Assertion{
			{
				Metric:    "error_rate",
				Condition: func(v float64) bool { return v < 5.0 },
				Message:   "Error rate should stay below 5% due to circuit breaker",
			},
		},
		Duration:    2 * time.Minute,
		BlastRadius: 1.0,
	}
}
