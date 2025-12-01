// cmd/api/main.go
package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

func main() {
	catalogServiceURL, _ := url.Parse(getEnv("CATALOG_SERVICE_URL", "http://localhost:8081"))
	circulationServiceURL, _ := url.Parse(getEnv("CIRCULATION_SERVICE_URL", "http://localhost:8082"))
	membershipServiceURL, _ := url.Parse(getEnv("MEMBERSHIP_SERVICE_URL", "http://localhost:8083"))

	catalogProxy := httputil.NewSingleHostReverseProxy(catalogServiceURL)
	circulationProxy := httputil.NewSingleHostReverseProxy(circulationServiceURL)
	membershipProxy := httputil.NewSingleHostReverseProxy(membershipServiceURL)

	http.Handle("/api/v1/catalog/", http.StripPrefix("/api/v1/catalog", catalogProxy))
	http.Handle("/api/v1/circulation/", http.StripPrefix("/api/v1/circulation", circulationProxy))
	http.Handle("/api/v1/members/", http.StripPrefix("/api/v1/members", membershipProxy))

	port := getEnv("PORT", "8080")
	log.Printf("API Gateway listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
