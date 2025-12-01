// internal/membership/domain.go
package membership

import (
	"time"

	"github.com/google/uuid"
)

// Member represents a library member.
type Member struct {
	ID             uuid.UUID `json:"id"`
	Email          string    `json:"email"`
	Name           string    `json:"name"`
	MembershipTier string    `json:"membership_tier"`
	Status         string    `json:"status"`
	FineBalance    float64   `json:"fine_balance"`
	MaxCheckouts   int       `json:"max_checkouts"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Version        int       `json:"version"`
}

// Credential represents a member's login credentials.
type Credential struct {
	MemberID      uuid.UUID `json:"member_id"`
	PasswordHash  string    `json:"-"`
	Salt          string    `json:"-"`
	MFAEnabled    bool      `json:"mfa_enabled"`
	MFASecret     string    `json:"-"`
	FailedAttempts int      `json:"-"`
	LockedUntil   time.Time `json:"-"`
}

// Event represents a domain event related to a member.
type Event struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// MemberRegisteredEvent is published when a new member registers.
type MemberRegisteredEvent struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}

// MemberTierChangedEvent is published when a member's tier is changed.
type MemberTierChangedEvent struct {
	ID      uuid.UUID `json:"id"`
	NewTier string    `json:"new_tier"`
}
