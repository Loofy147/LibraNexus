// cmd/circulation/main.go
package main

import (
	"database/sql"
	"fmt"
	"libranexus/internal/circulation"
	"libranexus/internal/clients"
	"libranexus/pkg/eventstore"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://libranexus:dev_password_change_in_prod@localhost:5432/libranexus?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	catalogServiceURL := os.Getenv("CATALOG_SERVICE_URL")
	if catalogServiceURL == "" {
		catalogServiceURL = "http://localhost:8081"
	}

	membershipServiceURL := os.Getenv("MEMBERSHIP_SERVICE_URL")
	if membershipServiceURL == "" {
		membershipServiceURL = "http://localhost:8083"
	}

	es := eventstore.NewEventStore(db)
	catalogClient := clients.NewCatalogClient(catalogServiceURL)
	membershipClient := clients.NewMembershipClient(membershipServiceURL)
	svc := circulation.NewService(es, db, catalogClient, membershipClient)
	handler := circulation.NewHandler(svc)

	router := http.NewServeMux()
	router.HandleFunc("/checkout", handler.HandleCheckout)
	router.HandleFunc("/return", handler.HandleReturn)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	fmt.Printf("🚀 Starting Circulation Service on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}
