// internal/catalog/service.go
package catalog

import (
	"context"

	"github.com/google/uuid"
)

// Service defines the interface for the catalog service.
type Service interface {
	AddItem(ctx context.Context, isbn, title, author string, totalCopies int) (*Item, error)
	GetItem(ctx context.Context, id uuid.UUID) (*Item, error)
	UpdateItemCopies(ctx context.Context, id uuid.UUID, newTotal, newAvailable int) error
	RemoveItem(ctx context.Context, id uuid.UUID) error
	Search(ctx context.Context, query string) ([]*Item, error)
}
