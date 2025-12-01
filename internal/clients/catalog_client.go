// internal/clients/catalog_client.go
package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"libranexus/internal/catalog"
	"net/http"

	"github.com/google/uuid"
)

type CatalogClient struct {
	baseURL string
}

func NewCatalogClient(baseURL string) *CatalogClient {
	return &CatalogClient{baseURL: baseURL}
}

func (c *CatalogClient) GetItem(ctx context.Context, id uuid.UUID) (*catalog.Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/items/%s", c.baseURL, id), nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var item catalog.Item
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, err
	}

	return &item, nil
}

func (c *CatalogClient) UpdateItemCopies(ctx context.Context, id uuid.UUID, newTotal, newAvailable int) error {
	updateReq := struct {
		TotalCopies int `json:"total_copies"`
		Available   int `json:"available"`
	}{
		TotalCopies: newTotal,
		Available:   newAvailable,
	}

	body, err := json.Marshal(updateReq)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, fmt.Sprintf("%s/items/%s", c.baseURL, id), bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (c *CatalogClient) Search(ctx context.Context, query string) ([]*catalog.Item, error) {
	// This is a placeholder and will not be used by the circulation service
	return nil, nil
}

func (c *CatalogClient) AddItem(ctx context.Context, isbn, title, author string, totalCopies int) (*catalog.Item, error) {
	// This is a placeholder and will not be used by the circulation service
	return nil, nil
}

func (c *CatalogClient) RemoveItem(ctx context.Context, id uuid.UUID) error {
	// This is a placeholder and will not be used by the circulation service
	return nil
}
