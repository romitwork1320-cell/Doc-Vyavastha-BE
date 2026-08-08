package templates

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
)

type Service struct {
	db *pgxpool.Pool
	q  *public.Queries
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{
		db: db,
		q:  public.New(db),
	}
}

// Document Type Logic

type DocumentTypeDto struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

func (s *Service) CreateDocumentType(ctx context.Context, dto DocumentTypeDto) (public.DocumentType, error) {
	return s.q.CreateDocumentType(ctx, public.CreateDocumentTypeParams{
		Name:        dto.Name,
		Description: dto.Description,
	})
}

func (s *Service) ListDocumentTypes(ctx context.Context) ([]public.DocumentType, error) {
	return s.q.ListDocumentTypes(ctx)
}

func (s *Service) UpdateDocumentType(ctx context.Context, id int64, dto DocumentTypeDto) (public.DocumentType, error) {
	return s.q.UpdateDocumentType(ctx, public.UpdateDocumentTypeParams{
		ID:          id,
		Name:        dto.Name,
		Description: dto.Description,
	})
}

func (s *Service) DeleteDocumentType(ctx context.Context, id int64) error {
	return s.q.DeleteDocumentType(ctx, id)
}

// Application Type Logic

type TemplateDto struct {
	Name        string                `json:"name"`
	Description *string               `json:"description"`
	Documents   []TemplateDocumentDto `json:"documents"`
}

type TemplateDocumentDto struct {
	DocumentTypeID int64 `json:"document_type_id"`
	DisplayOrder   int32 `json:"display_order"`
	IsRequired     bool  `json:"is_required"`
}

func (s *Service) CreateApplicationType(ctx context.Context, dto TemplateDto) (public.ApplicationType, error) {
	appType, err := s.q.CreateApplicationType(ctx, public.CreateApplicationTypeParams{
		Name:        dto.Name,
		Description: dto.Description,
	})
	if err != nil {
		return appType, err
	}

	for _, doc := range dto.Documents {
		err = s.q.AddDocumentToApplicationType(ctx, public.AddDocumentToApplicationTypeParams{
			ApplicationTypeID: appType.ID,
			DocumentTypeID:    doc.DocumentTypeID,
			DisplayOrder:      doc.DisplayOrder,
			IsRequired:        doc.IsRequired,
		})
		if err != nil {
			return appType, err
		}
	}

	return appType, nil
}

func (s *Service) UpdateApplicationType(ctx context.Context, id int64, dto TemplateDto) (public.ApplicationType, error) {
	appType, err := s.q.UpdateApplicationType(ctx, public.UpdateApplicationTypeParams{
		ID:          id,
		Name:        dto.Name,
		Description: dto.Description,
	})
	if err != nil {
		return appType, err
	}

	// Remove old documents
	err = s.q.RemoveDocumentsFromApplicationType(ctx, id)
	if err != nil {
		return appType, err
	}

	// Add new documents
	for _, doc := range dto.Documents {
		err = s.q.AddDocumentToApplicationType(ctx, public.AddDocumentToApplicationTypeParams{
			ApplicationTypeID: id,
			DocumentTypeID:    doc.DocumentTypeID,
			DisplayOrder:      doc.DisplayOrder,
			IsRequired:        doc.IsRequired,
		})
		if err != nil {
			return appType, err
		}
	}

	return appType, nil
}

func (s *Service) DeleteApplicationType(ctx context.Context, id int64) error {
	return s.q.DeleteApplicationType(ctx, id)
}

func (s *Service) ListApplicationTypes(ctx context.Context) ([]map[string]interface{}, error) {
	appTypes, err := s.q.ListApplicationTypes(ctx)
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for _, appType := range appTypes {
		docs, _ := s.q.GetDocumentsForApplicationType(ctx, appType.ID)
		results = append(results, map[string]interface{}{
			"ID":          appType.ID,
			"Name":        appType.Name,
			"Description": appType.Description,
			"CreatedAt":   appType.CreatedAt,
			"UpdatedAt":   appType.UpdatedAt,
			"Documents":   docs,
		})
	}

	// Ensure we return an empty array instead of null if empty
	if results == nil {
		results = []map[string]interface{}{}
	}

	return results, nil
}

func (s *Service) GetApplicationTypeWithDocuments(ctx context.Context, id int64) (map[string]interface{}, error) {
	appType, err := s.q.GetApplicationType(ctx, id)
	if err != nil {
		return nil, err
	}

	docs, err := s.q.GetDocumentsForApplicationType(ctx, id)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"application_type": appType,
		"documents":        docs,
	}, nil
}
