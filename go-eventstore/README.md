# go-eventstore

A simple, production-ready event store implementation for Go using PostgreSQL.

This package provides the core components for building event-sourced applications. It is extracted from the [LibraNexus](https://github.com/yourusername/libranexus) project, a reference implementation for distributed systems.

## When to Use Event Sourcing

Event sourcing is a powerful pattern, but it's not always the right choice. Consider using it when:

- **You need a full audit trail:** Every change to the system is an event, providing a complete, immutable history. This is invaluable for debugging, compliance, and business intelligence.
- **Your business logic is complex:** Event sourcing can help model complex workflows and state transitions more clearly than traditional CRUD models.
- **You need to support temporal queries:** You can reconstruct the state of an entity at any point in time.
- **You are building a distributed system:** Events are a natural way to communicate state changes between services.

However, be aware of the trade-offs:

- **Increased complexity:** Event sourcing requires a different way of thinking and can be more complex to implement than traditional state management.
- **Data migration can be challenging:** Evolving event schemas requires careful planning and versioning.
- **Read models are required:** You'll need to build and maintain projections of your event data for efficient querying.

## Advanced Usage Example

Here's a more detailed example demonstrating how to handle different event types, load events from a specific version, and use the snapshot functionality.

```go
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jules-labs/go-eventstore"
	_ "github.com/lib/pq"
)

// Define our events
type CheckoutStarted struct {
	MemberID uuid.UUID `json:"member_id"`
	ItemID   uuid.UUID `json:"item_id"`
}

type ItemReserved struct {
	ItemID uuid.UUID `json:"item_id"`
}

type CheckoutCompleted struct {
	CheckoutID uuid.UUID `json:"checkout_id"`
}

// A simple aggregate to demonstrate state reconstruction
type Checkout struct {
	ID        uuid.UUID
	MemberID  uuid.UUID
	ItemID    uuid.UUID
	IsReserved bool
	IsCompleted bool
	Version   int
}

// Apply applies an event to the Checkout aggregate, updating its state.
func (c *Checkout) Apply(event eventstore.Event) error {
	switch event.EventType {
	case "CheckoutStarted":
		var e CheckoutStarted
		if err := json.Unmarshal(event.EventData, &e); err != nil {
			return err
		}
		c.ID = event.AggregateID
		c.MemberID = e.MemberID
		c.ItemID = e.ItemID
	case "ItemReserved":
		c.IsReserved = true
	case "CheckoutCompleted":
		c.IsCompleted = true
	}
	c.Version = event.Version
	return nil
}

// NewCheckoutFromHistory reconstructs the state of a Checkout from its event history.
func NewCheckoutFromHistory(events []eventstore.Event) (*Checkout, error) {
	checkout := &Checkout{}
	for _, event := range events {
		if err := checkout.Apply(event); err != nil {
			return nil, err
		}
	}
	return checkout, nil
}

func main() {
	// 1. Connect to PostgreSQL
	db, err := sql.Open("postgres", "user=user password=password dbname=db sslmode=disable")
	if err != nil {
		panic(err)
	}

	// 2. Create a new event store
	store := eventstore.NewEventStore(db)
	ctx := context.Background()

	// 3. Create a new checkout stream
	aggregateID := uuid.New()
	aggregateType := "checkout"

	// 4. Append events in batches
	checkoutStartedData, _ := json.Marshal(CheckoutStarted{MemberID: uuid.New(), ItemID: uuid.New()})
	err = store.AppendEvents(ctx, aggregateID, aggregateType, 0, []eventstore.Event{
		{EventType: "CheckoutStarted", EventData: checkoutStartedData},
	})
	if err != nil {
		panic(err)
	}

	itemReservedData, _ := json.Marshal(ItemReserved{ItemID: uuid.New()})
	err = store.AppendEvents(ctx, aggregateID, aggregateType, 1, []eventstore.Event{
		{EventType: "ItemReserved", EventData: itemReservedData},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("Events appended successfully!")

	// 5. Load all events for the aggregate and reconstruct state
	allEvents, err := store.LoadEvents(ctx, aggregateID, 0, 0)
	if err != nil {
		panic(err)
	}
	checkout, err := NewCheckoutFromHistory(allEvents)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Reconstructed state: %+v\n", checkout)

	// 6. Load events from a specific version
	eventsSinceV1, err := store.LoadEvents(ctx, aggregateID, 1, 0)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Found %d events since version 1\n", len(eventsSinceV1))

	// 7. Create and save a snapshot
	snapshotData, _ := json.Marshal(checkout)
	snapshot := eventstore.Snapshot{
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		Version:       checkout.Version,
		State:         snapshotData,
	}
	if err := store.SaveSnapshot(ctx, snapshot); err != nil {
		panic(err)
	}
	fmt.Println("Snapshot saved successfully!")

	// 8. Load from snapshot and apply subsequent events
	loadedSnapshot, err := store.LoadSnapshot(ctx, aggregateID)
	if err != nil {
		panic(err)
	}
	if loadedSnapshot != nil {
		var checkoutFromSnapshot Checkout
		if err := json.Unmarshal(loadedSnapshot.State, &checkoutFromSnapshot); err != nil {
			panic(err)
		}

		eventsSinceSnapshot, err := store.LoadEvents(ctx, aggregateID, loadedSnapshot.Version, 0)
		if err != nil {
			panic(err)
		}

		for _, event := range eventsSinceSnapshot {
			checkoutFromSnapshot.Apply(event)
		}
		fmt.Printf("Reconstructed from snapshot: %+v\n", checkoutFromSnapshot)
	}
}
```

## Snapshotting

For aggregates with a large number of events, reconstructing the state from the beginning can become a performance bottleneck. To optimize this, `go-eventstore` provides support for snapshots.

A snapshot is a serialized representation of an aggregate's state at a specific version. When loading an aggregate, you can first load the latest snapshot and then apply only the events that have occurred *since* that snapshot, significantly reducing the number of events that need to be processed.

### `SaveSnapshot(ctx context.Context, snapshot Snapshot) error`
Saves a snapshot for a given aggregate. It will only overwrite an existing snapshot if the new one has a higher version number, preventing race conditions.

### `LoadSnapshot(ctx context.Context, aggregateID uuid.UUID) (*Snapshot, error)`
Loads the most recent snapshot for a given aggregate. If no snapshot is found, it returns `nil, nil`.
