# go-chaos

A simple chaos engineering framework for Go applications.

This package provides a lightweight, extensible framework for defining and running chaos experiments in your Go services. It is extracted from the [LibraNexus](https://github.com/yourusername/libranexus) project, a reference implementation for distributed systems.

## Philosophy

> "The best way to ensure high availability is to continuously break your system in controlled ways."

`go-chaos` allows you to codify your chaos experiments, making them repeatable, version-controlled, and part of your continuous integration pipeline.

## Usage Example

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jules-labs/go-chaos"
	_ "github.com/lib/pq"
)

func main() {
	// 1. Connect to your database (or other infrastructure)
	db, err := sql.Open("postgres", "user=user password=password dbname=db sslmode=disable")
	if err != nil {
		panic(err)
	}

	// 2. Create a new chaos engine
	engine := chaos.NewChaosEngine(db)

	// 3. Define a chaos experiment
	experiment := chaos.ChaosExperiment{
		Name:       "database-latency-injection",
		Hypothesis: "System degrades gracefully when database latency exceeds threshold",
		Method: []chaos.Action{
			{
				Type:   "inject-latency",
				Target: "postgres-primary",
				Parameters: map[string]interface{}{
					"latency": 250 * time.Millisecond,
				},
				Execute: func(ctx context.Context) error {
					fmt.Println("Injecting 250ms of latency into database calls...")
					// Your logic to inject latency would go here
					return nil
				},
			},
		},
		Validation: []chaos.Assertion{
			{
				Metric: "checkout_success_rate",
				Condition: func(v float64) bool { return v > 95.0 },
				Message:   "Checkout success rate should remain above 95%",
			},
		},
		Duration: 1 * time.Minute,
	}

	// 4. Register and run the experiment
	engine.RegisterExperiment(experiment)
	results, err := engine.RunExperiment(context.Background(), "database-latency-injection")
	if err != nil {
		panic(err)
	}

	// 5. Analyze the results
	if results.Success {
		fmt.Println("Experiment succeeded: Hypothesis confirmed!")
	} else {
		fmt.Println("Experiment failed: Hypothesis rejected.")
		for _, failure := range results.Failures {
			fmt.Printf(" - Validation failed: %s\n", failure)
		}
	}
}
```