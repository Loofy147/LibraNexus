package eventstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// setupTestDB attempts to connect to a PostgreSQL database for testing.
// It skips the test if the connection cannot be established.
func setupTestDB(t testing.TB) *sql.DB {
	t.Helper()

	pgUser := os.Getenv("PGUSER")
	pgPassword := os.Getenv("PGPASSWORD")
	pgHost := os.Getenv("PGHOST")
	pgPort := os.Getenv("PGPORT")
	pgDB := os.Getenv("PGDATABASE")

	if pgUser == "" {
		pgUser = "user"
	}
	if pgPassword == "" {
		pgPassword = "password"
	}
	if pgHost == "" {
		pgHost = "localhost"
	}
	if pgPort == "" {
		pgPort = "5432"
	}
	if pgDB == "" {
		pgDB = "testdb"
	}

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		pgHost, pgPort, pgUser, pgPassword, pgDB)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("failed to open database connection: %v", err)
	}

	if err := db.Ping(); err != nil {
		t.Skipf("skipping benchmark tests: could not connect to postgres: %v", err)
	}

	// Simple schema for testing
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id BIGSERIAL PRIMARY KEY,
			aggregate_id UUID NOT NULL,
			aggregate_type TEXT NOT NULL,
			event_type TEXT NOT NULL,
			event_data JSONB NOT NULL,
			metadata JSONB,
			version INT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (aggregate_id, version)
		);
	`)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

type TestEvent struct {
	Message string `json:"message"`
}

func BenchmarkAppendEvents(b *testing.B) {
	db := setupTestDB(b)
	defer db.Close()
	store := NewEventStore(db)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer() // Stop timer for setup
		aggregateID := uuid.New()
		aggregateType := "test_aggregate"
		eventData, _ := json.Marshal(TestEvent{Message: fmt.Sprintf("event %d", i)})
		events := []Event{
			{
				EventType: "TestEvent",
				EventData: eventData,
			},
		}
		b.StartTimer() // Resume timer for the operation

		err := store.AppendEvents(context.Background(), aggregateID, aggregateType, 0, events)
		if err != nil {
			b.Fatalf("AppendEvents failed: %v", err)
		}
	}
}

func BenchmarkLoadEvents(b *testing.B) {
	db := setupTestDB(b)
	defer db.Close()
	store := NewEventStore(db)

	// Setup: create an aggregate with 10 events
	aggregateID := uuid.New()
	aggregateType := "test_aggregate"
	for i := 0; i < 10; i++ {
		eventData, _ := json.Marshal(TestEvent{Message: fmt.Sprintf("event %d", i)})
		events := []Event{
			{
				EventType: "TestEvent",
				EventData: eventData,
			},
		}
		err := store.AppendEvents(context.Background(), aggregateID, aggregateType, i, events)
		if err != nil {
			b.Fatalf("failed to setup events for benchmark: %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := store.LoadEvents(context.Background(), aggregateID, 0, 0)
		if err != nil {
			b.Fatalf("LoadEvents failed: %v", err)
		}
	}
}
