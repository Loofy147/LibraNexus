// internal/membership/implementation.go
package membership

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/jules-labs/go-eventstore"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// service implements the Service interface.
type service struct {
	eventStore   *eventstore.EventStore
	db           *sql.DB
	rateLimiter  *rate.Limiter
}

// NewService creates a new membership service instance.
func NewService(es *eventstore.EventStore, db *sql.DB) Service {
	return &service{
		eventStore:   es,
		db:           db,
		rateLimiter:  rate.NewLimiter(rate.Every(1*time.Minute), 5), // 5 requests per minute
	}
}

// RegisterMember creates a new member.
func (s *service) RegisterMember(ctx context.Context, email, name, password string) (*Member, error) {
	if !s.rateLimiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	id := uuid.New()
	passwordHash, salt, err := hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	eventData := MemberRegisteredEvent{
		ID:    id,
		Email: email,
		Name:  name,
	}

	jsonData, err := json.Marshal(eventData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event data: %w", err)
	}

	event := eventstore.Event{
		AggregateID:   id,
		AggregateType: "member",
		EventType:     "MemberRegistered",
		EventData:     jsonData,
		Version:       1,
	}

	if err := s.eventStore.AppendEvents(ctx, id, "member", 0, []eventstore.Event{event}); err != nil {
		return nil, fmt.Errorf("failed to append event: %w", err)
	}

	member := &Member{
		ID:             id,
		Email:          email,
		Name:           name,
		MembershipTier: "basic",
		Status:         "active",
		ExpiresAt:      time.Now().AddDate(1, 0, 0),
	}
	credential := &Credential{
		MemberID:     id,
		PasswordHash: passwordHash,
		Salt:         salt,
	}

	if err := s.insertMemberIntoReadModel(ctx, member, credential); err != nil {
		return nil, fmt.Errorf("failed to update read model: %w", err)
	}

	return member, nil
}

func (s *service) insertMemberIntoReadModel(ctx context.Context, member *Member, credential *Credential) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	memberQuery := `
		INSERT INTO members (id, email, name, membership_tier, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err = tx.ExecContext(ctx, memberQuery, member.ID, member.Email, member.Name, member.MembershipTier, member.Status, member.ExpiresAt)
	if err != nil {
		return err
	}

	credQuery := `
		INSERT INTO credentials (member_id, password_hash, salt)
		VALUES ($1, $2, $3)
	`
	_, err = tx.ExecContext(ctx, credQuery, credential.MemberID, credential.PasswordHash, credential.Salt)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Authenticate verifies a member's credentials and returns the member if successful.
func (s *service) Authenticate(ctx context.Context, email, password string) (*Member, error) {
	if !s.rateLimiter.Allow() {
		return nil, fmt.Errorf("rate limit exceeded")
	}

	member, err := s.getMemberByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	credential, err := s.getCredentialByMemberID(ctx, member.ID)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	ok, err := verifyPassword(password, credential.Salt, credential.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("authentication failed: invalid credentials")
	}

	return member, nil
}

func (s *service) getMemberByEmail(ctx context.Context, email string) (*Member, error) {
	query := `
		SELECT id, email, name, membership_tier, status, expires_at
		FROM members
		WHERE email = $1
	`
	member := &Member{}
	err := s.db.QueryRowContext(ctx, query, email).Scan(
		&member.ID,
		&member.Email,
		&member.Name,
		&member.MembershipTier,
		&member.Status,
		&member.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return member, nil
}

func (s *service) getCredentialByMemberID(ctx context.Context, memberID uuid.UUID) (*Credential, error) {
	query := `
		SELECT member_id, password_hash, salt
		FROM credentials
		WHERE member_id = $1
	`
	credential := &Credential{}
	err := s.db.QueryRowContext(ctx, query, memberID).Scan(
		&credential.MemberID,
		&credential.PasswordHash,
		&credential.Salt,
	)
	if err != nil {
		return nil, err
	}
	return credential, nil
}

// GetMember retrieves a member by their ID.
func (s *service) GetMember(ctx context.Context, id uuid.UUID) (*Member, error) {
	query := `
		SELECT id, email, name, membership_tier, status, version, expires_at
		FROM members
		WHERE id = $1
	`
	member := &Member{}
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&member.ID,
		&member.Email,
		&member.Name,
		&member.MembershipTier,
		&member.Status,
		&member.Version,
		&member.ExpiresAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("member with ID %s not found", id)
		}
		return nil, fmt.Errorf("failed to get member from read model: %w", err)
	}

	return member, nil
}

// UpdateMemberTier updates a member's membership tier.
func (s *service) UpdateMemberTier(ctx context.Context, id uuid.UUID, newTier string) error {
	member, err := s.GetMember(ctx, id)
	if err != nil {
		return err
	}

	eventData := MemberTierChangedEvent{
		ID:      id,
		NewTier: newTier,
	}

	jsonData, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	event := eventstore.Event{
		AggregateID:   id,
		AggregateType: "member",
		EventType:     "MemberTierChanged",
		EventData:     jsonData,
		Version:       member.Version + 1,
	}

	if err := s.eventStore.AppendEvents(ctx, id, "member", member.Version, []eventstore.Event{event}); err != nil {
		return fmt.Errorf("failed to append event: %w", err)
	}

	// Update read model
	query := `
		UPDATE members
		SET membership_tier = $1, version = $2, updated_at = NOW()
		WHERE id = $3
	`
	_, err = s.db.ExecContext(ctx, query, newTier, member.Version+1, id)
	return err
}
