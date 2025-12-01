// tests/integration/main_test.go
package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"libranexus/internal/catalog"
	"libranexus/internal/circulation"
	"libranexus/internal/membership"
	"net/http"
	"os/exec"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestSuite struct {
	db *sql.DB
}

func setupTestSuite(t *testing.T) *TestSuite {
	cmd := exec.Command("sudo", "docker", "compose", "down", "-v", "--remove-orphans")
	cmd.Run()

	cmd = exec.Command("sudo", "docker", "compose", "up", "-d")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("docker compose up output:\n%s", string(output))
	}
	require.NoError(t, err)

	time.Sleep(20 * time.Second)

	var db *sql.DB
	for i := 0; i < 5; i++ {
		db, err = sql.Open("postgres", "postgres://libranexus:dev_password_change_in_prod@localhost:5432/libranexus?sslmode=disable")
		if err == nil {
			err = db.Ping()
			if err == nil {
				break
			}
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)

	_, err = db.Exec("TRUNCATE TABLE events, items, checkouts, members, credentials CASCADE")
	require.NoError(t, err)

	return &TestSuite{db: db}
}

func (ts *TestSuite) teardown() {
	ts.db.Close()
	cmd := exec.Command("sudo", "docker", "compose", "down", "-v", "--remove-orphans")
	cmd.Run()
}

func TestCheckoutFlow(t *testing.T) {
	ts := setupTestSuite(t)
	defer ts.teardown()

	// Register a new member
	member := &membership.Member{}
	registerReq := map[string]string{"email": "test@example.com", "name": "Test User", "password": "SecurePass123!"}
	body, _ := json.Marshal(registerReq)
	resp, err := http.Post("http://localhost:8080/api/v1/members/register", "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	json.NewDecoder(resp.Body).Decode(member)

	// Add a new item
	item := &catalog.Item{}
	addReq := map[string]interface{}{"isbn": "9780141439518", "title": "Pride and Prejudice", "author": "Jane Austen", "total_copies": 5}
	body, _ = json.Marshal(addReq)
	resp, err = http.Post("http://localhost:8080/api/v1/catalog/items", "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	json.NewDecoder(resp.Body).Decode(item)

	// Checkout the item
	checkout := &circulation.Checkout{}
	checkoutReq := map[string]string{"member_id": member.ID.String(), "item_id": item.ID.String()}
	body, _ = json.Marshal(checkoutReq)
	resp, err = http.Post("http://localhost:8080/api/v1/circulation/checkout", "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	json.NewDecoder(resp.Body).Decode(checkout)

	// Verify item availability
	resp, err = http.Get(fmt.Sprintf("http://localhost:8080/api/v1/catalog/items/%s", item.ID))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var updatedItem catalog.Item
	json.NewDecoder(resp.Body).Decode(&updatedItem)
	assert.Equal(t, 4, updatedItem.Available)

	// Return the item
	returnReq := map[string]string{"member_id": member.ID.String(), "item_id": item.ID.String()}
	body, _ = json.Marshal(returnReq)
	resp, err = http.Post("http://localhost:8080/api/v1/circulation/return", "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify item availability
	resp, err = http.Get(fmt.Sprintf("http://localhost:8080/api/v1/catalog/items/%s", item.ID))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	json.NewDecoder(resp.Body).Decode(&updatedItem)
	assert.Equal(t, 5, updatedItem.Available)
}

func TestConcurrentCheckoutPreventsDoubleBooking(t *testing.T) {
	ts := setupTestSuite(t)
	defer ts.teardown()

	// Add a new item with 1 copy
	item := &catalog.Item{}
	addReq := map[string]interface{}{"isbn": "9780743273565", "title": "The Great Gatsby", "author": "F. Scott Fitzgerald", "total_copies": 1}
	body, _ := json.Marshal(addReq)
	resp, err := http.Post("http://localhost:8080/api/v1/catalog/items", "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	json.NewDecoder(resp.Body).Decode(item)

	// Register multiple members
	var members []*membership.Member
	for i := 0; i < 10; i++ {
		member := &membership.Member{}
		registerReq := map[string]string{"email": fmt.Sprintf("member%d@test.com", i), "name": fmt.Sprintf("Member %d", i), "password": "SecurePass123!"}
		body, _ := json.Marshal(registerReq)
		resp, err := http.Post("http://localhost:8080/api/v1/members/register", "application/json", bytes.NewBuffer(body))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		json.NewDecoder(resp.Body).Decode(member)
		members = append(members, member)
	}

	// Attempt concurrent checkouts
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for _, member := range members {
		wg.Add(1)
		go func(m *membership.Member) {
			defer wg.Done()
			checkoutReq := map[string]string{"member_id": m.ID.String(), "item_id": item.ID.String()}
			body, _ := json.Marshal(checkoutReq)
			resp, err := http.Post("http://localhost:8080/api/v1/circulation/checkout", "application/json", bytes.NewBuffer(body))
			if err == nil && resp.StatusCode == http.StatusCreated {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(member)
	}

	wg.Wait()

	assert.Equal(t, 1, successCount, "Only one concurrent checkout should succeed")

	// Verify item availability
	resp, err = http.Get(fmt.Sprintf("http://localhost:8080/api/v1/catalog/items/%s", item.ID))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var updatedItem catalog.Item
	json.NewDecoder(resp.Body).Decode(&updatedItem)
	assert.Equal(t, 0, updatedItem.Available)
}
