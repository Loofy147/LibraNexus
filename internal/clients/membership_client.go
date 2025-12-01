// internal/clients/membership_client.go
package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"libranexus/internal/membership"
	"net/http"

	"github.com/google/uuid"
)

type MembershipClient struct {
	baseURL string
}

func NewMembershipClient(baseURL string) *MembershipClient {
	return &MembershipClient{baseURL: baseURL}
}

func (c *MembershipClient) GetMember(ctx context.Context, id uuid.UUID) (*membership.Member, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/members/%s", c.baseURL, id), nil)
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

	var member membership.Member
	if err := json.NewDecoder(resp.Body).Decode(&member); err != nil {
		return nil, err
	}

	return &member, nil
}

func (c *MembershipClient) RegisterMember(ctx context.Context, email, name, password string) (*membership.Member, error) {
	// This is a placeholder and will not be used by the circulation service
	return nil, nil
}

func (c *MembershipClient) Authenticate(ctx context.Context, email, password string) (*membership.Member, error) {
	// This is a placeholder and will not be used by the circulation service
	return nil, nil
}

func (c *MembershipClient) UpdateMemberTier(ctx context.Context, id uuid.UUID, newTier string) error {
	// This is a placeholder and will not be used by the circulation service
	return nil
}
