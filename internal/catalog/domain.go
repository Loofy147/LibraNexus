// internal/catalog/domain.go
package catalog

import (
	"time"

	"github.com/google/uuid"
)

// Item represents a book or other library item.
type Item struct {
	ID             uuid.UUID `json:"id"`
	ISBN           string    `json:"isbn"`
	Title          string    `json:"title"`
	Author         string    `json:"author"`
	Publisher      string    `json:"publisher,omitempty"`
	PublishedYear  int       `json:"published_year,omitempty"`
	TotalCopies    int       `json:"total_copies"`
	Available      int       `json:"available"`
	Status         string    `json:"status"`
	Version        int       `json:"version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Event represents a domain event related to a catalog item.
type Event struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// ItemAddedEvent is published when a new item is added.
type ItemAddedEvent struct {
	ID            uuid.UUID `json:"id"`
	ISBN          string    `json:"isbn"`
	Title         string    `json:"title"`
	Author        string    `json:"author"`
	TotalCopies   int       `json:"total_copies"`
}

// ItemCopiesUpdatedEvent is published when the number of copies changes.
type ItemCopiesUpdatedEvent struct {
	ID          uuid.UUID `json:"id"`
	NewTotal    int       `json:"new_total"`
	NewAvailable int      `json:"new_available"`
}

// ItemRemovedEvent is published when an item is retired from the catalog.
type ItemRemovedEvent struct {
	ID     uuid.UUID `json:"id"`
	Status string    `json:"status"`
}
