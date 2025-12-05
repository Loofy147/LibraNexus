// cmd/chaos/main.go
package main

import (
	"context"
	"database/sql"
	"github.com/jules-labs/go-chaos"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://libranexus:dev_password_change_in_prod@localhost:5432/libranexus?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	engine := chaos.NewChaosEngine(db)
	engine.RegisterExperiments()

	gameDay := chaos.GameDay{
		Name:      "Weekly Chaos Game Day",
		Date:      time.Now(),
		Scenarios: engine.GetExperiments(),
	}

	if err := engine.ExecuteGameDay(context.Background(), gameDay); err != nil {
		log.Fatalf("Chaos Game Day failed: %v", err)
	}
}
