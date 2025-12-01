// internal/circulation/service.go
package circulation

import (
	"context"

	"github.com/google/uuid"
)

// Service defines the interface for the circulation service.
type Service interface {
	CheckoutItem(ctx context.Context, memberID, itemID uuid.UUID) (*Checkout, error)
	ReturnItem(ctx context.Context, memberID, itemID uuid.UUID) error
}
