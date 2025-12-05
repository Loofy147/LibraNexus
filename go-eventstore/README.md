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

## Usage Example

Here's a simplified example of how to use the event store, based on the checkout saga from LibraNexus.

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
	MemberID uuid.UUID
	ItemID   uuid.UUID
}

type ItemReserved struct {
	ItemID uuid.UUID
}

type CheckoutCompleted struct {
	CheckoutID uuid.UUID
}

func main() {
	// 1. Connect to PostgreSQL
	db, err := sql.Open("postgres", "user=user password=password dbname=db sslmode=disable")
	if err != nil {
		panic(err)
	}

	// 2. Create a new event store
	store := eventstore.NewEventStore(db)

	// 3. Create a new checkout stream
	checkoutID := uuid.New()
	aggregateID := checkoutID
	aggregateType := "checkout"

	// 4. Append events to the stream
	checkoutStartedData, _ := json.Marshal(CheckoutStarted{MemberID: uuid.New(), ItemID: uuid.New()})
	itemReservedData, _ := json.Marshal(ItemReserved{ItemID: uuid.New()})
	checkoutCompletedData, _ := json.Marshal(CheckoutCompleted{CheckoutID: checkoutID})

	events := []eventstore.Event{
		{
			EventType: "CheckoutStarted",
			EventData: checkoutStartedData,
		},
		{
			EventType: "ItemReserved",
			EventData: itemReservedData,
		},
		{
			EventType: "CheckoutCompleted",
			EventData: checkoutCompletedData,
		},
	}

	ctx := context.Background()
	err = store.AppendEvents(ctx, aggregateID, aggregateType, 0, events)
	if err != nil {
		panic(err)
	}

	fmt.Println("Events appended successfully!")

	// 5. Read events from the stream
	readEvents, err := store.LoadEvents(ctx, aggregateID, 0, 0)
	if err != nil {
		panic(err)
	}

	for _, event := range readEvents {
		fmt.Printf("Read event: Type=%s, Timestamp=%s\n", event.EventType, event.CreatedAt.Format(time.RFC3339))
	}
}
```