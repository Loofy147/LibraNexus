// internal/catalog/implementation.go
package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"libranexus/pkg/eventstore"

	"github.com/google/uuid"
)

// service implements the Service interface.
type service struct {
	eventStore *eventstore.EventStore
	db         *sql.DB
}

// NewService creates a new catalog service instance.
func NewService(es *eventstore.EventStore, db *sql.DB) Service {
	return &service{
		eventStore: es,
		db:         db,
	}
}

// AddItem creates a new item in the catalog.
func (s *service) AddItem(ctx context.Context, isbn, title, author string, totalCopies int) (*Item, error) {
	id := uuid.New()
	eventData := ItemAddedEvent{
		ID:          id,
		ISBN:        isbn,
		Title:       title,
		Author:      author,
		TotalCopies: totalCopies,
	}

	jsonData, err := json.Marshal(eventData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event data: %w", err)
	}

	event := eventstore.Event{
		AggregateID:   id,
		AggregateType: "item",
		EventType:     "ItemAdded",
		EventData:     jsonData,
		Version:       1,
	}

	// Append the event to the event store
	if err := s.eventStore.AppendEvents(ctx, id, "item", 0, []eventstore.Event{event}); err != nil {
		return nil, fmt.Errorf("failed to append event: %w", err)
	}

	// Update the read model (this would typically be done by a separate projector)
	item := &Item{
		ID:          id,
		ISBN:        isbn,
		Title:       title,
		Author:      author,
		TotalCopies: totalCopies,
		Available:   totalCopies,
		Status:      "active",
		Version:     1,
	}
	if err := s.insertItemIntoReadModel(ctx, item); err != nil {
		return nil, fmt.Errorf("failed to update read model: %w", err)
	}

	return item, nil
}

func (s *service) insertItemIntoReadModel(ctx context.Context, item *Item) error {
	query := `
		INSERT INTO items (id, isbn, title, author, total_copies, available, status, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := s.db.ExecContext(ctx, query, item.ID, item.ISBN, item.Title, item.Author, item.TotalCopies, item.Available, item.Status, item.Version)
	return err
}

// GetItem retrieves an item from the catalog by its ID.
func (s *service) GetItem(ctx context.Context, id uuid.UUID) (*Item, error) {
	query := `
		SELECT id, isbn, title, author, total_copies, available, status, version, created_at, updated_at
		FROM items
		WHERE id = $1
	`
	item := &Item{}
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&item.ID,
		&item.ISBN,
		&item.Title,
		&item.Author,
		&item.TotalCopies,
		&item.Available,
		&item.Status,
		&item.Version,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("item with ID %s not found", id)
		}
		return nil, fmt.Errorf("failed to get item from read model: %w", err)
	}

	return item, nil
}

// UpdateItemCopies updates the number of copies for an item.
func (s *service) UpdateItemCopies(ctx context.Context, id uuid.UUID, newTotal, newAvailable int) error {
	item, err := s.GetItem(ctx, id)
	if err != nil {
		return err
	}

	eventData := ItemCopiesUpdatedEvent{
		ID:           id,
		NewTotal:     newTotal,
		NewAvailable: newAvailable,
	}

	jsonData, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	event := eventstore.Event{
		AggregateID:   id,
		AggregateType: "item",
		EventType:     "ItemCopiesUpdated",
		EventData:     jsonData,
		Version:       item.Version + 1,
	}

	if err := s.eventStore.AppendEvents(ctx, id, "item", item.Version, []eventstore.Event{event}); err != nil {
		return fmt.Errorf("failed to append event: %w", err)
	}

	// Update read model
	query := `
		UPDATE items
		SET total_copies = $1, available = $2, version = version + 1, updated_at = NOW()
		WHERE id = $3 AND version = $4
	`
	_, err = s.db.ExecContext(ctx, query, newTotal, newAvailable, id, item.Version)
	return err
}

// RemoveItem marks an item as retired.
func (s *service) RemoveItem(ctx context.Context, id uuid.UUID) error {
	item, err := s.GetItem(ctx, id)
	if err != nil {
		return err
	}

	eventData := ItemRemovedEvent{
		ID:     id,
		Status: "retired",
	}

	jsonData, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	event := eventstore.Event{
		AggregateID:   id,
		AggregateType: "item",
		EventType:     "ItemRemoved",
		EventData:     jsonData,
		Version:       item.Version + 1,
	}

	if err := s.eventStore.AppendEvents(ctx, id, "item", item.Version, []eventstore.Event{event}); err != nil {
		return fmt.Errorf("failed to append event: %w", err)
	}

	// Update read model
	query := `
		UPDATE items
		SET status = 'retired', version = version + 1, updated_at = NOW()
		WHERE id = $1 AND version = $2
	`
	_, err = s.db.ExecContext(ctx, query, id, item.Version)
	return err
}

// Search finds items in the catalog.
func (s *service) Search(ctx context.Context, query string) ([]*Item, error) {
	return s.searchDatabase(ctx, query)
}

func (s *service) searchDatabase(ctx context.Context, query string) ([]*Item, error) {
	dbQuery := `
		SELECT id, isbn, title, author, total_copies, available, status
		FROM items
		WHERE to_tsvector('english', title) @@ to_tsquery('english', $1)
		OR to_tsvector('english', author) @@ to_tsquery('english', $1)
		LIMIT 10
	`
	rows, err := s.db.QueryContext(ctx, dbQuery, query)
	if err != nil {
		return nil, fmt.Errorf("database search failed: %w", err)
	}
	defer rows.Close()

	var items []*Item
	for rows.Next() {
		item := &Item{}
		if err := rows.Scan(&item.ID, &item.ISBN, &item.Title, &item.Author, &item.TotalCopies, &item.Available, &item.Status); err != nil {
			return nil, fmt.Errorf("failed to scan item: %w", err)
		}
		items = append(items, item)
	}

	return items, nil
}
