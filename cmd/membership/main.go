// cmd/membership/main.go
package main

import (
	"database/sql"
	"fmt"
	"libranexus/internal/membership"
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
	svc := membership.NewService(es, db)
	handler := membership.NewHandler(svc)

	router := http.NewServeMux()
	router.HandleFunc("/members", handler.HandleMembers)
	router.HandleFunc("/members/", handler.HandleMember)
	router.HandleFunc("/login", handler.HandleLogin)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}

	fmt.Printf("🚀 Starting Membership Service on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}
