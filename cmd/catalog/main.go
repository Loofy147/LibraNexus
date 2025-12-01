// cmd/catalog/main.go
package main

import (
	"database/sql"
	"fmt"
	"libranexus/internal/catalog"
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

	es := eventstore.NewEventStore(db)
	svc := catalog.NewService(es, db)
	handler := catalog.NewHandler(svc)

	router := http.NewServeMux()
	router.HandleFunc("/items", handler.HandleItems)
	router.HandleFunc("/items/", handler.HandleItem)
	router.HandleFunc("/search", handler.HandleSearch)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	fmt.Printf("🚀 Starting Catalog Service on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}
