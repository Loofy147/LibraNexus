// internal/membership/service.go
package membership

import (
	"context"

	"github.com/google/uuid"
)

// Service defines the interface for the membership service.
type Service interface {
	RegisterMember(ctx context.Context, email, name, password string) (*Member, error)
	Authenticate(ctx context.Context, email, password string) (*Member, error)
	GetMember(ctx context.Context, id uuid.UUID) (*Member, error)
	UpdateMemberTier(ctx context.Context, id uuid.UUID, newTier string) error
}
