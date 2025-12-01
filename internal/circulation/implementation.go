// internal/circulation/implementation.go
package circulation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"libranexus/internal/clients"
	"libranexus/pkg/eventstore"
	"log"
	"time"

	"github.com/google/uuid"
)

// service implements the Service interface.
type service struct {
	eventStore      *eventstore.EventStore
	db              *sql.DB
	catalogClient   *clients.CatalogClient
	membershipClient *clients.MembershipClient
}

// NewService creates a new circulation service instance.
func NewService(es *eventstore.EventStore, db *sql.DB, catalogClient *clients.CatalogClient, membershipClient *clients.MembershipClient) Service {
	return &service{
		eventStore:      es,
		db:              db,
		catalogClient:   catalogClient,
		membershipClient: membershipClient,
	}
}

// CheckoutItem orchestrates the checkout saga.
func (s *service) CheckoutItem(ctx context.Context, memberID, itemID uuid.UUID) (*Checkout, error) {
	// Step 1: Validate the member
	member, err := s.membershipClient.GetMember(ctx, memberID)
	if err != nil {
		return nil, fmt.Errorf("failed to get member: %w", err)
	}
	if member.Status != "active" || member.FineBalance > 0 {
		return nil, fmt.Errorf("member is not eligible for checkout")
	}

	// Step 2: Check item availability
	item, err := s.catalogClient.GetItem(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get item: %w", err)
	}
	if item.Available <= 0 {
		return nil, fmt.Errorf("item is not available")
	}

	// Step 3: Decrement item availability (with compensation)
	err = s.catalogClient.UpdateItemCopies(ctx, itemID, item.TotalCopies, item.Available-1)
	if err != nil {
		return nil, fmt.Errorf("failed to update item copies: %w", err)
	}

	// Compensation function for decrementing item availability
	compensation := func() {
		log.Printf("Compensating for failed checkout: rolling back item availability for item %s", itemID)
		if err := s.catalogClient.UpdateItemCopies(ctx, itemID, item.TotalCopies, item.Available); err != nil {
			log.Printf("Failed to compensate item availability: %v", err)
		}
	}

	// Step 4: Create the checkout record
	checkoutID := uuid.New()
	dueDate := time.Now().AddDate(0, 0, 14) // 2 weeks

	eventData := ItemCheckedOutEvent{
		CheckoutID: checkoutID,
		MemberID:   memberID,
		ItemID:     itemID,
		DueDate:    dueDate,
	}
	jsonData, err := json.Marshal(eventData)
	if err != nil {
		compensation()
		return nil, fmt.Errorf("failed to marshal event data: %w", err)
	}

	event := eventstore.Event{
		AggregateID:   checkoutID,
		AggregateType: "checkout",
		EventType:     "ItemCheckedOut",
		EventData:     jsonData,
		Version:       1,
	}

	if err := s.eventStore.AppendEvents(ctx, checkoutID, "checkout", 0, []eventstore.Event{event}); err != nil {
		compensation()
		return nil, fmt.Errorf("failed to append event: %w", err)
	}

	// Step 5: Update the read model
	checkout := &Checkout{
		ID:           checkoutID,
		MemberID:     memberID,
		ItemID:       itemID,
		CheckoutDate: time.Now(),
		DueDate:      dueDate,
		Status:       "active",
	}

	if err := s.insertCheckoutIntoReadModel(ctx, checkout); err != nil {
		compensation()
		return nil, fmt.Errorf("failed to update read model: %w", err)
	}

	return checkout, nil
}

func (s *service) insertCheckoutIntoReadModel(ctx context.Context, checkout *Checkout) error {
	query := `
		INSERT INTO checkouts (id, member_id, item_id, checkout_date, due_date, status)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := s.db.ExecContext(ctx, query, checkout.ID, checkout.MemberID, checkout.ItemID, checkout.CheckoutDate, checkout.DueDate, checkout.Status)
	return err
}

// ReturnItem handles returning an item.
func (s *service) ReturnItem(ctx context.Context, memberID, itemID uuid.UUID) error {
	// Step 1: Find the active checkout
	checkout, err := s.getActiveCheckout(ctx, memberID, itemID)
	if err != nil {
		return fmt.Errorf("failed to find active checkout: %w", err)
	}

	// Step 2: Increment item availability
	item, err := s.catalogClient.GetItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("failed to get item: %w", err)
	}
	err = s.catalogClient.UpdateItemCopies(ctx, itemID, item.TotalCopies, item.Available+1)
	if err != nil {
		return fmt.Errorf("failed to update item copies: %w", err)
	}

	// Step 3: Create the return event
	eventData := ItemReturnedEvent{
		CheckoutID: checkout.ID,
		MemberID:   memberID,
		ItemID:     itemID,
		ReturnDate: time.Now(),
	}
	jsonData, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	event := eventstore.Event{
		AggregateID:   checkout.ID,
		AggregateType: "checkout",
		EventType:     "ItemReturned",
		EventData:     jsonData,
		Version:       checkout.Version + 1,
	}

	if err := s.eventStore.AppendEvents(ctx, checkout.ID, "checkout", checkout.Version, []eventstore.Event{event}); err != nil {
		// If appending the event fails, we should compensate by decrementing the item availability
		log.Printf("Failed to append return event, compensating item availability for item %s", itemID)
		if err := s.catalogClient.UpdateItemCopies(ctx, itemID, item.TotalCopies, item.Available); err != nil {
			log.Printf("Failed to compensate item availability: %v", err)
		}
		return fmt.Errorf("failed to append event: %w", err)
	}

	// Step 4: Update the read model
	query := `
		UPDATE checkouts
		SET status = 'returned', return_date = $1, updated_at = NOW()
		WHERE id = $2
	`
	_, err = s.db.ExecContext(ctx, query, time.Now(), checkout.ID)
	return err
}

func (s *service) getActiveCheckout(ctx context.Context, memberID, itemID uuid.UUID) (*Checkout, error) {
	query := `
		SELECT id, version
		FROM checkouts
		WHERE member_id = $1 AND item_id = $2 AND status = 'active'
	`
	checkout := &Checkout{}
	err := s.db.QueryRowContext(ctx, query, memberID, itemID).Scan(&checkout.ID, &checkout.Version)
	if err != nil {
		return nil, err
	}
	return checkout, nil
}
