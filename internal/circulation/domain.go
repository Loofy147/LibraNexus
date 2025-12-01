// internal/circulation/domain.go
package circulation

import (
	"time"

	"github.com/google/uuid"
)

// Checkout represents an item checked out by a member.
type Checkout struct {
	ID           uuid.UUID `json:"id"`
	MemberID     uuid.UUID `json:"member_id"`
	ItemID       uuid.UUID `json:"item_id"`
	CheckoutDate time.Time `json:"checkout_date"`
	DueDate      time.Time `json:"due_date"`
	ReturnDate   time.Time `json:"return_date,omitempty"`
	Status       string    `json:"status"`
	Version      int       `json:"version"`
}

// Event represents a domain event related to circulation.
type Event struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// ItemCheckedOutEvent is published when an item is checked out.
type ItemCheckedOutEvent struct {
	CheckoutID uuid.UUID `json:"checkout_id"`
	MemberID   uuid.UUID `json:"member_id"`
	ItemID     uuid.UUID `json:"item_id"`
	DueDate    time.Time `json:"due_date"`
}

// ItemReturnedEvent is published when an item is returned.
type ItemReturnedEvent struct {
	CheckoutID uuid.UUID `json:"checkout_id"`
	MemberID   uuid.UUID `json:"member_id"`
	ItemID     uuid.UUID `json:"item_id"`
	ReturnDate time.Time `json:"return_date"`
}
