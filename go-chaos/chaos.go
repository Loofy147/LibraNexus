// chaos/chaos.go
package chaos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ChaosExperiment defines a chaos engineering test
type ChaosExperiment struct {
	Name        string
	Hypothesis  string
	SteadyState []Metric
	Method      []Action
	Rollback    []Action
	Validation  []Assertion
	Duration    time.Duration
	BlastRadius float64 // 0.0 to 1.0 (percentage of system affected)
}

// Metric defines a measurable system property
type Metric struct {
	Name      string
	Query     func(context.Context) (float64, error)
	Threshold Threshold
}

type Threshold struct {
	Operator string  // >, <, >=, <=, ==
	Value    float64
}

// Action represents a fault injection or recovery action
type Action struct {
	Type       string                 // latency, failure, partition, resource_exhaustion
	Target     string                 // service/component name
	Parameters map[string]interface{}
	Execute    func(context.Context) error
}

// Assertion validates experiment outcome
type Assertion struct {
	Metric    string
	Condition func(float64) bool
	Message   string
}

// ExperimentResult captures experiment execution data
type ExperimentResult struct {
	ExperimentName   string                 `json:"experiment_name"`
	StartTime        time.Time              `json:"start_time"`
	EndTime          time.Time              `json:"end_time"`
	Duration         time.Duration          `json:"duration"`
	HypothesisHeld   bool                   `json:"hypothesis_held"`
	SteadyStateValid bool                   `json:"steady_state_valid"`
	Violations       []MetricViolation      `json:"violations"`
	Observations     map[string][]DataPoint `json:"observations"`
	ErrorEvents      []ErrorEvent           `json:"error_events"`
	MTTR             *time.Duration         `json:"mttr,omitempty"`
}

type MetricViolation struct {
	MetricName string    `json:"metric_name"`
	Expected   float64   `json:"expected"`
	Actual     float64   `json:"actual"`
	Timestamp  time.Time `json:"timestamp"`
}

type DataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type ErrorEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error"`
	Component string    `json:"component"`
}

// ChaosEngine orchestrates chaos experiments
type ChaosEngine struct {
	tracer      trace.Tracer
	db          *sql.DB
	experiments []ChaosExperiment
	results     []ExperimentResult
	mu          sync.Mutex
}

func NewChaosEngine(db *sql.DB) *ChaosEngine {
	return &ChaosEngine{
		tracer:      otel.Tracer("github.com/jules-labs/go-chaos"),
		db:          db,
		experiments: make([]ChaosExperiment, 0),
		results:     make([]ExperimentResult, 0),
	}
}

// RegisterExperiment adds an experiment to the test suite
func (ce *ChaosEngine) RegisterExperiment(exp ChaosExperiment) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.experiments = append(ce.experiments, exp)
}

// GetExperiments returns the list of registered experiments.
func (ce *ChaosEngine) GetExperiments() []ChaosExperiment {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	return ce.experiments
}

// RunExperiment executes a single chaos experiment
func (ce *ChaosEngine) RunExperiment(ctx context.Context, exp ChaosExperiment) (*ExperimentResult, error) {
	ctx, span := ce.tracer.Start(ctx, "chaos.run_experiment",
		trace.WithAttributes(
			attribute.String("experiment.name", exp.Name),
		),
	)
	defer span.End()

	result := &ExperimentResult{
		ExperimentName: exp.Name,
		StartTime:      time.Now(),
		Observations:   make(map[string][]DataPoint),
		ErrorEvents:    make([]ErrorEvent, 0),
	}

	// Phase 1: Validate steady state
	span.AddEvent("validating_steady_state")
	if valid, violations := ce.validateSteadyState(ctx, exp.SteadyState); !valid {
		result.SteadyStateValid = false
		result.Violations = violations
		return result, errors.New("steady state invalid - aborting experiment")
	}
	result.SteadyStateValid = true

	// Phase 2: Inject chaos
	span.AddEvent("injecting_chaos")
	for _, action := range exp.Method {
		if err := action.Execute(ctx); err != nil {
			result.ErrorEvents = append(result.ErrorEvents, ErrorEvent{
				Timestamp: time.Now(),
				Error:     err.Error(),
				Component: action.Target,
			})
			span.RecordError(err)
		}
	}

	// Phase 3: Observe system behavior
	span.AddEvent("observing_system")
	observationCtx, cancel := context.WithTimeout(ctx, exp.Duration)
	defer cancel()

	recoveryStart := time.Time{}
	systemRecovered := false

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-observationCtx.Done():
			goto ROLLBACK
		case <-ticker.C:
			// Sample metrics
			for _, metric := range exp.SteadyState {
				value, err := metric.Query(ctx)
				if err != nil {
					result.ErrorEvents = append(result.ErrorEvents, ErrorEvent{
						Timestamp: time.Now(),
						Error:     err.Error(),
						Component: metric.Name,
					})
					continue
				}

				result.Observations[metric.Name] = append(
					result.Observations[metric.Name],
					DataPoint{Timestamp: time.Now(), Value: value},
				)

				// Check if system violates threshold
				if !ce.evaluateThreshold(value, metric.Threshold) {
					if recoveryStart.IsZero() {
						recoveryStart = time.Now()
					}
					result.Violations = append(result.Violations, MetricViolation{
						MetricName: metric.Name,
						Expected:   metric.Threshold.Value,
						Actual:     value,
						Timestamp:  time.Now(),
					})
				} else if !recoveryStart.IsZero() && !systemRecovered {
					// System recovered
					mttr := time.Since(recoveryStart)
					result.MTTR = &mttr
					systemRecovered = true
				}
			}
		}
	}

ROLLBACK:
	// Phase 4: Rollback chaos injection
	span.AddEvent("rolling_back")
	for _, action := range exp.Rollback {
		if err := action.Execute(ctx); err != nil {
			span.RecordError(err)
		}
	}

	// Phase 5: Validate assertions
	span.AddEvent("validating_assertions")
	result.HypothesisHeld = ce.validateAssertions(ctx, exp.Validation, result)
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	ce.mu.Lock()
	ce.results = append(ce.results, *result)
	ce.mu.Unlock()

	span.SetAttributes(
		attribute.Bool("hypothesis_held", result.HypothesisHeld),
		attribute.Int("violations", len(result.Violations)),
	)

	return result, nil
}

func (ce *ChaosEngine) validateSteadyState(ctx context.Context, metrics []Metric) (bool, []MetricViolation) {
	violations := make([]MetricViolation, 0)

	for _, metric := range metrics {
		value, err := metric.Query(ctx)
		if err != nil {
			violations = append(violations, MetricViolation{
				MetricName: metric.Name,
				Expected:   metric.Threshold.Value,
				Actual:     -1,
				Timestamp:  time.Now(),
			})
			continue
		}

		if !ce.evaluateThreshold(value, metric.Threshold) {
			violations = append(violations, MetricViolation{
				MetricName: metric.Name,
				Expected:   metric.Threshold.Value,
				Actual:     value,
				Timestamp:  time.Now(),
			})
		}
	}

	return len(violations) == 0, violations
}

func (ce *ChaosEngine) evaluateThreshold(value float64, threshold Threshold) bool {
	switch threshold.Operator {
	case ">":
		return value > threshold.Value
	case "<":
		return value < threshold.Value
	case ">=":
		return value >= threshold.Value
	case "<=":
		return value <= threshold.Value
	case "==":
		return value == threshold.Value
	default:
		return false
	}
}

func (ce *ChaosEngine) validateAssertions(ctx context.Context, assertions []Assertion, result *ExperimentResult) bool {
	for _, assertion := range assertions {
		observations, exists := result.Observations[assertion.Metric]
		if !exists {
			return false
		}

		// Get final observation
		if len(observations) == 0 {
			return false
		}

		finalValue := observations[len(observations)-1].Value
		if !assertion.Condition(finalValue) {
			return false
		}
	}

	return true
}

// GameDay orchestrates a series of chaos experiments.
type GameDay struct {
	Name        string
	Date        time.Time
	Scenarios   []ChaosExperiment
	Participants []string
	Runbooks    map[string]string
}

func (ce *ChaosEngine) ExecuteGameDay(ctx context.Context, gameDay GameDay) error {
	ctx, span := ce.tracer.Start(ctx, "chaos.game_day",
		trace.WithAttributes(
			attribute.String("gameday.name", gameDay.Name),
		),
	)
	defer span.End()

	fmt.Printf("🎮 Starting Game Day: %s\n", gameDay.Name)
	fmt.Printf("📅 Date: %s\n", gameDay.Date)
	fmt.Printf("👥 Participants: %v\n", gameDay.Participants)

	for i, scenario := range gameDay.Scenarios {
		fmt.Printf("\n🔬 Experiment %d/%d: %s\n", i+1, len(gameDay.Scenarios), scenario.Name)
		fmt.Printf("💡 Hypothesis: %s\n", scenario.Hypothesis)

		result, err := ce.RunExperiment(ctx, scenario)
		if err != nil {
			fmt.Printf("❌ Experiment failed: %v\n", err)
			continue
		}

		ce.printExperimentResult(result)

		// Wait between experiments
		time.Sleep(30 * time.Second)
	}

	return nil
}

func (ce *ChaosEngine) printExperimentResult(result *ExperimentResult) {
	if result.HypothesisHeld {
		fmt.Printf("✅ Hypothesis held - System behaved as expected\n")
	} else {
		fmt.Printf("❌ Hypothesis violated - Unexpected behavior observed\n")
	}

	if len(result.Violations) > 0 {
		fmt.Printf("⚠️  Violations detected: %d\n", len(result.Violations))
		for _, v := range result.Violations {
			fmt.Printf("   - %s: expected %.2f, got %.2f\n", v.MetricName, v.Expected, v.Actual)
		}
	}

	if result.MTTR != nil {
		fmt.Printf("⏱️  MTTR: %s\n", *result.MTTR)
	}

	fmt.Printf("📊 Duration: %s\n", result.Duration)
}
