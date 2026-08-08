package magiclink

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thinkparq/edconsultancy-be/internal/db/public"
)

// Service handles magic link generation and validation
type Service struct {
	queries *public.Queries
}

// NewService creates a new MagicLink Service
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		queries: public.New(pool),
	}
}

// GenerateLink creates a new secure magic link token for a specific application.
// It automatically invalidates any previously unused links for this application.
func (s *Service) GenerateLink(ctx context.Context, tenantID int64, appID int64, clientID int64) (string, error) {
	// First, invalidate old magic links for this application
	err := s.queries.InvalidateMagicLinksForApplication(ctx, public.InvalidateMagicLinksForApplicationParams{
		ApplicationID: appID,
		TenantID:      tenantID,
	})
	if err != nil && err != pgx.ErrNoRows {
		return "", err
	}

	// Generate a secure UUID
	token := uuid.New().String()
	
	// Set expiration to 7 days
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	_, err = s.queries.CreateMagicLink(ctx, public.CreateMagicLinkParams{
		Token:         uuid.MustParse(token),
		TenantID:      tenantID,
		ApplicationID: appID,
		ClientID:      clientID,
		ExpiresAt:     expiresAt,
	})
	
	if err != nil {
		return "", err
	}

	return token, nil
}

// ValidateLink verifies if a token is valid, hasn't expired, and hasn't been used (if applicable).
// In this workflow, magic links can be used multiple times until they expire, 
// so we don't necessarily mark it as "used" unless we want a one-time link.
// Since clients can return to upload more docs, we only check expiration.
func (s *Service) ValidateLink(ctx context.Context, token string) (public.MagicLink, error) {
	parsedToken, err := uuid.Parse(token)
	if err != nil {
		return public.MagicLink{}, err
	}

	link, err := s.queries.GetMagicLinkByToken(ctx, parsedToken)
	if err != nil {
		return public.MagicLink{}, err
	}

	// Check if invalidated by regeneration
	if link.IsUsed {
		return public.MagicLink{}, pgx.ErrNoRows // Treat as invalid
	}

	// Check expiration
	if time.Now().After(link.ExpiresAt) {
		return public.MagicLink{}, pgx.ErrNoRows // Treat as invalid/expired
	}

	return link, nil
}
