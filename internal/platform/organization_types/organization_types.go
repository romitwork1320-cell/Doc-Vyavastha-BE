package organization_types

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

type OrganizationTypeDto struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Service) ListOrganizationTypes(ctx context.Context) ([]public.OrganizationType, error) {
	q := public.New(s.pool)
	return q.ListOrganizationTypes(ctx)
}

func (s *Service) CreateOrganizationType(ctx context.Context, dto OrganizationTypeDto) (public.OrganizationType, error) {
	var desc *string
	if dto.Description != "" {
		desc = &dto.Description
	}
	q := public.New(s.pool)
	return q.CreateOrganizationType(ctx, public.CreateOrganizationTypeParams{
		Name:        dto.Name,
		Description: desc,
	})
}

func (s *Service) UpdateOrganizationType(ctx context.Context, id int64, dto OrganizationTypeDto) (public.OrganizationType, error) {
	var desc *string
	if dto.Description != "" {
		desc = &dto.Description
	}
	q := public.New(s.pool)
	return q.UpdateOrganizationType(ctx, public.UpdateOrganizationTypeParams{
		ID:          id,
		Name:        dto.Name,
		Description: desc,
	})
}

func (s *Service) DeleteOrganizationType(ctx context.Context, id int64) error {
	q := public.New(s.pool)
	return q.DeleteOrganizationType(ctx, id)
}
