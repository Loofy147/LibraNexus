package eventstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var (
	ErrConcurrencyConflict = errors.New("concurrency conflict: version mismatch")
	ErrAggregateNotFound   = errors.New("aggregate not found")
	ErrInvalidVersion      = errors.New("invalid version number")
)

// Event represents a domain event with full metadata
type Event struct {
	ID            int64                  `json:"id" db:"id"`
	AggregateID   uuid.UUID              `json:"aggregate_id" db:"aggregate_id"`
	AggregateType string                 `json:"aggregate_type" db:"aggregate_type"`
	EventType     string                 `json:"event_type" db:"event_type"`
	EventData     json.RawMessage        `json:"event_data" db:"event_data"`
	Metadata      map[string]interface{} `json:"metadata" db:"metadata"`
	Version       int                    `json:"version" db:"version"`
	CreatedAt     time.Time              `json:"created_at" db:"created_at"`
}

// EventStore provides ACID guarantees for event sourcing
type EventStore struct {
	db     *sql.DB
	tracer trace.Tracer
}

// NewEventStore creates a new event store with connection pooling
func NewEventStore(db *sql.DB) *EventStore {
	return &EventStore{
		db:     db,
		tracer: otel.Tracer("libranexus/eventstore"),
	}
}

// AppendEvents atomically appends events with optimistic concurrency control
func (es *EventStore) AppendEvents(ctx context.Context, aggregateID uuid.UUID, aggregateType string, expectedVersion int, events []Event) error {
	ctx, span := es.tracer.Start(ctx, "eventstore.append",
		trace.WithAttributes(
			attribute.String("aggregate.id", aggregateID.String()),
			attribute.String("aggregate.type", aggregateType),
			attribute.Int("expected.version", expectedVersion),
			attribute.Int("event.count", len(events)),
		),
	)
	defer span.End()

	// Begin transaction with serializable isolation
	tx, err := es.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify current version
	var currentVersion int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version), 0)
		FROM events
		WHERE aggregate_id = $1
	`, aggregateID).Scan(&currentVersion)

	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("query current version: %w", err)
	}

	// Optimistic concurrency check
	if currentVersion != expectedVersion {
		span.SetAttributes(
			attribute.Int("actual.version", currentVersion),
			attribute.Bool("conflict.detected", true),
		)
		return ErrConcurrencyConflict
	}

	// Insert events atomically
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events (aggregate_id, aggregate_type, event_type, event_data, metadata, version, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for i, event := range events {
		version := expectedVersion + i + 1
		metadataJSON, _ := json.Marshal(event.Metadata)

		var eventID int64
		err = stmt.QueryRowContext(
			ctx,
			aggregateID,
			aggregateType,
			event.EventType,
			event.EventData,
			metadataJSON,
			version,
			time.Now().UTC(),
		).Scan(&eventID)

		if err != nil {
			// Check for unique constraint violation (race condition)
			if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
				return ErrConcurrencyConflict
			}
			return fmt.Errorf("insert event %d: %w", i, err)
		}

		span.AddEvent("event.appended", trace.WithAttributes(
			attribute.Int64("event.id", eventID),
			attribute.Int("event.version", version),
			attribute.String("event.type", event.EventType),
		))
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	span.SetAttributes(attribute.Bool("append.success", true))
	return nil
}

// LoadEvents retrieves all events for an aggregate with optional version range
func (es *EventStore) LoadEvents(ctx context.Context, aggregateID uuid.UUID, fromVersion, toVersion int) ([]Event, error) {
	ctx, span := es.tracer.Start(ctx, "eventstore.load",
		trace.WithAttributes(
			attribute.String("aggregate.id", aggregateID.String()),
			attribute.Int("from.version", fromVersion),
			attribute.Int("to.version", toVersion),
		),
	)
	defer span.End()

	query := `
		SELECT id, aggregate_id, aggregate_type, event_type, event_data, metadata, version, created_at
		FROM events
		WHERE aggregate_id = $1
		AND version >= $2
	`

	args := []interface{}{aggregateID, fromVersion}

	if toVersion > 0 {
		query += " AND version <= $3"
		args = append(args, toVersion)
	}

	query += " ORDER BY version ASC"

	rows, err := es.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		var metadataJSON []byte

		err := rows.Scan(
			&event.ID,
			&event.AggregateID,
			&event.AggregateType,
			&event.EventType,
			&event.EventData,
			&metadataJSON,
			&event.Version,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		if len(metadataJSON) > 0 {
			json.Unmarshal(metadataJSON, &event.Metadata)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}

	span.SetAttributes(attribute.Int("events.loaded", len(events)))
	return events, nil
}

// GetCurrentVersion returns the latest version for an aggregate
func (es *EventStore) GetCurrentVersion(ctx context.Context, aggregateID uuid.UUID) (int, error) {
	ctx, span := es.tracer.Start(ctx, "eventstore.get_version",
		trace.WithAttributes(
			attribute.String("aggregate.id", aggregateID.String()),
		),
	)
	defer span.End()

	var version int
	err := es.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version), 0)
		FROM events
		WHERE aggregate_id = $1
	`, aggregateID).Scan(&version)

	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("query version: %w", err)
	}

	span.SetAttributes(attribute.Int("current.version", version))
	return version, nil
}

// StreamEvents provides a cursor-based event stream for projections
func (es *EventStore) StreamEvents(ctx context.Context, fromID int64, batchSize int) ([]Event, error) {
	ctx, span := es.tracer.Start(ctx, "eventstore.stream",
		trace.WithAttributes(
			attribute.Int64("from.id", fromID),
			attribute.Int("batch.size", batchSize),
		),
	)
	defer span.End()

	rows, err := es.db.QueryContext(ctx, `
		SELECT id, aggregate_id, aggregate_type, event_type, event_data, metadata, version, created_at
		FROM events
		WHERE id > $1
		ORDER BY id ASC
		LIMIT $2
	`, fromID, batchSize)

	if err != nil {
		return nil, fmt.Errorf("query event stream: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		var metadataJSON []byte

		err := rows.Scan(
			&event.ID,
			&event.AggregateID,
			&event.AggregateType,
			&event.EventType,
			&event.EventData,
			&metadataJSON,
			&event.Version,
			&event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		if len(metadataJSON) > 0 {
			json.Unmarshal(metadataJSON, &event.Metadata)
		}

		events = append(events, event)
	}

	span.SetAttributes(attribute.Int("events.streamed", len(events)))
	return events, nil
}

// Snapshot support for performance optimization
type Snapshot struct {
	AggregateID   uuid.UUID       `json:"aggregate_id"`
	AggregateType string          `json:"aggregate_type"`
	Version       int             `json:"version"`
	State         json.RawMessage `json:"state"`
	CreatedAt     time.Time       `json:"created_at"`
}

// SaveSnapshot stores aggregate state for faster reconstitution
func (es *EventStore) SaveSnapshot(ctx context.Context, snapshot Snapshot) error {
	ctx, span := es.tracer.Start(ctx, "eventstore.save_snapshot")
	defer span.End()

	_, err := es.db.ExecContext(ctx, `
		INSERT INTO snapshots (aggregate_id, aggregate_type, version, state, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (aggregate_id) DO UPDATE
		SET version = EXCLUDED.version,
		    state = EXCLUDED.state,
		    created_at = EXCLUDED.created_at
		WHERE snapshots.version < EXCLUDED.version
	`, snapshot.AggregateID, snapshot.AggregateType, snapshot.Version, snapshot.State, time.Now().UTC())

	if err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}

	return nil
}

// LoadSnapshot retrieves the latest snapshot for fast replay
func (es *EventStore) LoadSnapshot(ctx context.Context, aggregateID uuid.UUID) (*Snapshot, error) {
	ctx, span := es.tracer.Start(ctx, "eventstore.load_snapshot")
	defer span.End()

	var snapshot Snapshot
	err := es.db.QueryRowContext(ctx, `
		SELECT aggregate_id, aggregate_type, version, state, created_at
		FROM snapshots
		WHERE aggregate_id = $1
	`, aggregateID).Scan(
		&snapshot.AggregateID,
		&snapshot.AggregateType,
		&snapshot.Version,
		&snapshot.State,
		&snapshot.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // No snapshot exists
	}
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}

	return &snapshot, nil
}