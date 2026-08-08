package document_types

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
)

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool: pool,
	}
}

func (s *Service) ListDocumentTypes(ctx context.Context) ([]public.DocumentType, error) {
	q := public.New(s.pool)
	return q.ListDocumentTypes(ctx)
}

type DocumentTypeDto struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Service) CreateDocumentType(ctx context.Context, dto DocumentTypeDto) (public.DocumentType, error) {
	var desc *string
	if dto.Description != "" {
		desc = &dto.Description
	}
	q := public.New(s.pool)
	return q.CreateDocumentType(ctx, public.CreateDocumentTypeParams{
		Name:        dto.Name,
		Description: desc,
	})
}

func (s *Service) UpdateDocumentType(ctx context.Context, id int64, dto DocumentTypeDto) (public.DocumentType, error) {
	var desc *string
	if dto.Description != "" {
		desc = &dto.Description
	}
	q := public.New(s.pool)
	return q.UpdateDocumentType(ctx, public.UpdateDocumentTypeParams{
		ID:          id,
		Name:        dto.Name,
		Description: desc,
	})
}

func (s *Service) DeleteDocumentType(ctx context.Context, id int64) error {
	q := public.New(s.pool)
	return q.DeleteDocumentType(ctx, id)
}

