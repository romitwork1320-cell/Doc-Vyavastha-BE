package applications

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/thinkparq/edconsultancy-be/internal/db/public"
	"github.com/thinkparq/edconsultancy-be/internal/db/tenant"
	"github.com/thinkparq/edconsultancy-be/internal/reqctx"
	"github.com/thinkparq/edconsultancy-be/internal/tenancy"
)

// Service provides application and requirement management logic.
type Service struct {
	tm *tenancy.Manager
}

// NewService creates a new Applications service.
func NewService(tm *tenancy.Manager) *Service {
	return &Service{
		tm: tm,
	}
}

// ApplicationDto is used to create a new application along with initial requirements.
type ApplicationDto struct {
	ClientID     int64            `json:"clientId"`
	Title        string           `json:"title"`
	Requirements []RequirementDto `json:"requirements"`
}

// RequirementDto defines a required document.
type RequirementDto struct {
	DocumentTypeID int64  `json:"document_type_id"`
	Description    string `json:"description"`
	DisplayOrder   int32  `json:"display_order"`
	IsRequired     bool   `json:"is_required"`
}

// CreateApplication creates a new application and its associated document requirements.
func (s *Service) CreateApplication(ctx context.Context, tenantID int64, dto ApplicationDto) (tenant.Application, error) {
	var app tenant.Application
	err := s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		var err error
		app, err = q.CreateApplication(ctx, tenant.CreateApplicationParams{
			ClientID: dto.ClientID,
			Title:    dto.Title,
		})
		if err != nil {
			return err
		}

		for _, req := range dto.Requirements {
			var desc *string
			if req.Description != "" {
				d := req.Description
				desc = &d
			}

			// Fetch the document type to capture its name as a historical snapshot
			docType, err := s.tm.Pub().GetDocumentType(ctx, req.DocumentTypeID)
			if err != nil {
				return fmt.Errorf("invalid document_type_id %d: %w", req.DocumentTypeID, err)
			}

			_, err = q.CreateApplicationRequirement(ctx, tenant.CreateApplicationRequirementParams{
				ApplicationID:  app.ID,
				DocumentTypeID: req.DocumentTypeID,
				DocumentName:   docType.Name, // Snapshot
				Description:    desc,
				DisplayOrder:   req.DisplayOrder,
				IsRequired:     req.IsRequired,
			})
			if err != nil {
				return fmt.Errorf("failed to create requirement %d: %w", req.DocumentTypeID, err)
			}
		}

		userID := reqctx.MustUserID(ctx)
		var actorID *int64
		if userID > 0 {
			actorID = &userID
		}

		_, err = q.CreateTimelineEvent(ctx, tenant.CreateTimelineEventParams{
			ApplicationID: app.ID,
			EventType:     "APPLICATION_CREATED",
			ActorType:     "ORGANIZATION",
			ActorID:       actorID,
			Description:   "Application created",
			Metadata:      []byte("{}"),
		})
		if err != nil {
			return fmt.Errorf("failed to create timeline event: %w", err)
		}

		return nil
	})
	return app, err
}

// ClientApplication extends tenant.Application with tenant info for the client portal.
type ClientApplication struct {
	tenant.Application
	TenantID   int64  `json:"tenantId"`
	TenantName string `json:"tenantName"`
}

// ListApplications retrieves applications. If tenantID is 0, it aggregates applications across all connected tenants for the client.
func (s *Service) ListApplications(ctx context.Context, tenantID int64, clientID int64) (interface{}, error) {
	if tenantID == 0 && clientID > 0 {
		var aggregatedApps []ClientApplication
		conns, err := s.tm.Pub().GetConnectedOrganizationsForClient(ctx, clientID)
		if err != nil {
			return nil, err
		}
		for _, conn := range conns {
			err = s.tm.InTenantTxByID(ctx, conn.TenantID, func(q *tenant.Queries) error {
				apps, err := q.ListApplicationsByClient(ctx, clientID)
				if err == nil {
					for _, app := range apps {
						tName := ""
						if conn.TenantName != nil {
							tName = *conn.TenantName
						}
						aggregatedApps = append(aggregatedApps, ClientApplication{
							Application: app,
							TenantID:    conn.TenantID,
							TenantName:  tName,
						})
					}
				}
				return nil // continue on error
			})
		}
		return aggregatedApps, nil
	}

	var apps []tenant.Application
	err := s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		var err error
		if clientID > 0 {
			apps, err = q.ListApplicationsByClient(ctx, clientID)
		} else {
			apps, err = q.ListApplications(ctx)
		}
		return err
	})
	return apps, err
}

// GetApplicationWithDetails fetches an application and all its requirements/versions.
func (s *Service) GetApplicationWithDetails(ctx context.Context, tenantID int64, appID int64) (map[string]interface{}, error) {
	var res map[string]interface{}
	err := s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		app, err := q.GetApplicationByID(ctx, appID)
		if err != nil {
			return err
		}

		// Enforce access control: check if the organization is blocked
		isBlocked := false
		
		// Only block if the requester is the organization
		if reqctx.TenantID(ctx) != 0 {
			conn, err := s.tm.Pub().GetClientConnectionByTenantAndClient(ctx, public.GetClientConnectionByTenantAndClientParams{
				TenantID: tenantID,
				ClientID: app.ClientID,
			})
			if err == nil && conn.Status == "BLOCKED" {
				isBlocked = true
			}
		}

		reqs, err := q.GetApplicationRequirements(ctx, appID)
		if err != nil {
			return err
		}

		var requirementsDetails []map[string]interface{}
		for _, r := range reqs {
			versions, _ := q.GetDocumentVersionsByRequirement(ctx, r.ID)
			reviews, _ := q.GetRequirementReviews(ctx, r.ID)

			var descStr string
			if r.Description != nil {
				descStr = *r.Description
			}
			var rejStr string
			if r.RejectionReason != nil {
				rejStr = *r.RejectionReason
			}

			var enrichedVersions []map[string]interface{}
			for _, v := range versions {
				var files []tenant.ApplicationDocumentVersionFile
				if !isBlocked {
					files, _ = q.GetVersionFilesByVersionID(ctx, v.ID)
				}
				enrichedVersions = append(enrichedVersions, map[string]interface{}{
					"id":                 v.ID,
					"requirement_id":     v.RequirementID,
					"version_number":     v.VersionNumber,
					"uploaded_by_client": v.UploadedByClient,
					"created_at":         v.CreatedAt,
					"files":              files,
				})
			}

			requirementsDetails = append(requirementsDetails, map[string]interface{}{
				"id":               r.ID,
				"document_name":    r.DocumentName,
				"description":      descStr,
				"status":           r.Status,
				"rejection_reason": rejStr,
				"created_at":       r.CreatedAt,
				"versions":         enrichedVersions,
				"reviews":          reviews,
			})
		}

		var deliverables []map[string]interface{}
		if !isBlocked {
			deliverablesRaw, _ := q.GetApplicationDeliverables(ctx, appID)
			for _, d := range deliverablesRaw {
				// Fetch client document details from public DB
				doc, err := s.tm.Pub().GetClientDocument(ctx, d.ClientDocumentID)
				if err != nil {
					continue
				}
				
				// Try to get uploader user info
				var uploaderName string
				if d.UploadedByUserID != nil {
					profile, err := s.tm.Pub().GetUserProfile(ctx, *d.UploadedByUserID)
					if err == nil {
						if profile.FirstName != nil {
							uploaderName = *profile.FirstName
						}
						if profile.LastName != nil {
							uploaderName += " " + *profile.LastName
						}
						uploaderName = strings.TrimSpace(uploaderName)
					}
					if uploaderName == "" {
						uploaderName = "Advocate"
					}
				} else {
					uploaderName = "Client"
				}

				deliverables = append(deliverables, map[string]interface{}{
					"id":               d.ID,
					"document_name":    doc.FileName,
					"uploaded_by":      uploaderName,
					"created_at":       d.CreatedAt,
					"client_document_id": d.ClientDocumentID,
				})
			}
		}

		res = map[string]interface{}{
			"application":  app,
			"requirements": requirementsDetails,
			"deliverables": deliverables,
		}
		return nil
	})
	return res, err
}

// UploadRequirementVersion handles uploading files for a specific requirement.
// It creates a new submission (version) containing all provided files.
func (s *Service) UploadRequirementVersion(ctx context.Context, tenantID int64, reqID int64, files []*multipart.FileHeader, isClient bool) (tenant.ApplicationDocumentVersion, error) {
	var version tenant.ApplicationDocumentVersion
	err := s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		req, err := q.GetApplicationRequirementByID(ctx, reqID)
		if err != nil {
			return fmt.Errorf("requirement not found: %w", err)
		}

		app, err := q.GetApplicationByID(ctx, req.ApplicationID)
		if err != nil {
			return err
		}
		if err := ensureEditable(app); err != nil {
			return err
		}

		// Calculate the next version number
		latestVersion, err := q.GetLatestDocumentVersionNumber(ctx, reqID)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		nextVersion := int32(latestVersion + 1)

		// Create version record
		version, err = q.CreateApplicationDocumentVersion(ctx, tenant.CreateApplicationDocumentVersionParams{
			RequirementID:    reqID,
			VersionNumber:    nextVersion,
			UploadedByClient: isClient,
		})
		if err != nil {
			return err
		}

		// The core business rule states that the Client Vault is the only source of truth.
		// Documents belong to the client, never the application.
		// Therefore, we upload the file directly into the client's vault directory.
		uploadDir := filepath.Join("uploads", "clients", fmt.Sprintf("%d", app.ClientID), "documents")
		if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
			return err
		}

		// Save the files to the vault
		for _, file := range files {
			ext := filepath.Ext(file.Filename)
			// Generate a unique physical filename in the vault
			filename := fmt.Sprintf("vault_%d_%s%s", app.ClientID, uuid.New().String()[:8], ext)

			dstPath := filepath.Join(uploadDir, filename)
			dst, err := os.Create(dstPath)
			if err != nil {
				return err
			}

			f, err := file.Open()
			if err != nil {
				dst.Close()
				return err
			}

			if _, err := io.Copy(dst, f); err != nil {
				f.Close()
				dst.Close()
				return err
			}
			f.Close()
			dst.Close()

			reqVersionFile, err := q.CreateRequirementVersionFile(ctx, tenant.CreateRequirementVersionFileParams{
				VersionID:    version.ID,
				FileUrl:      dstPath,
				DocumentName: file.Filename,
			})
			if err != nil {
				return err
			}

			// IMMEDIATELY create the Vault Document (ClientDocument)
			var fileSize int64 = 0
			mimeType := "application/octet-stream"
			if fileInfo, err := os.Stat(dstPath); err == nil {
				fileSize = fileInfo.Size()
			}
			extLower := strings.ToLower(ext)
			switch extLower {
			case ".pdf": mimeType = "application/pdf"
			case ".jpg", ".jpeg": mimeType = "image/jpeg"
			case ".png": mimeType = "image/png"
			}
			
			var actorID *int64
			if uID := reqctx.MustUserID(ctx); uID > 0 { actorID = &uID }

			doc, err := s.tm.Pub().CreateClientDocument(ctx, public.CreateClientDocumentParams{
				ClientID:         app.ClientID,
				DocumentTypeID:   req.DocumentTypeID,
				FileName:         file.Filename,
				OriginalFileName: file.Filename,
				FileSize:         fileSize,
				MimeType:         mimeType,
				Sha256Hash:       nil,
				StoragePath:      dstPath,
				UploadedBy:       actorID,
				Source:           "DIRECT_UPLOAD",
			})
			if err == nil {
				// Link it to the Requirement Version File
				_ = q.UpdateVersionFileClientDocumentID(ctx, tenant.UpdateVersionFileClientDocumentIDParams{
					ID:               reqVersionFile.ID,
					ClientDocumentID: &doc.ID,
				})

				// Grant Access to the Organization
				conn, cerr := s.tm.Pub().GetClientConnectionByTenantAndClient(ctx, public.GetClientConnectionByTenantAndClientParams{
					TenantID: tenantID,
					ClientID: app.ClientID,
				})
				if cerr == nil {
					_, _ = s.tm.Pub().CreateDocumentAccessGrant(ctx, public.CreateDocumentAccessGrantParams{
						DocumentID:   doc.ID,
						ConnectionID: conn.ID,
					})
				}
			}
		}

		// Update requirement status
		status := "UPLOADED"
		if !isClient {
			status = "APPROVED" // if advocate uploads it, skip review phase
		}

		q.UpdateRequirementStatus(ctx, tenant.UpdateRequirementStatusParams{
			ID:              reqID,
			Status:          status,
			RejectionReason: nil, // Clear any previous rejection
		})

		userID := reqctx.MustUserID(ctx)
		var actorID *int64
		if userID > 0 {
			actorID = &userID
		}
		actorType := "CLIENT"
		if !isClient {
			actorType = "ORGANIZATION"
		}

		_, err = q.CreateTimelineEvent(ctx, tenant.CreateTimelineEventParams{
			ApplicationID: req.ApplicationID, EventType: "DOCUMENT_UPLOADED", ActorType: actorType, ActorID: actorID, Description: "Uploaded document for: " + req.DocumentName, Metadata: []byte("{}"),
		})

		return nil
	})
	return version, err
}

// UploadRequirementVersionFromVault attaches an existing vault document to a requirement.
func (s *Service) UploadRequirementVersionFromVault(ctx context.Context, tenantID int64, reqID int64, clientDocumentIDs []int64, isClient bool) (tenant.ApplicationDocumentVersion, error) {
	var version tenant.ApplicationDocumentVersion

	if len(clientDocumentIDs) == 0 {
		return version, fmt.Errorf("no vault documents provided")
	}

	// Fetch the client documents
	var docs []public.ClientDocument
	for _, id := range clientDocumentIDs {
		doc, err := s.tm.Pub().GetClientDocument(ctx, id)
		if err != nil {
			return version, fmt.Errorf("vault document %d not found: %w", id, err)
		}
		docs = append(docs, doc)
	}

	err := s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		req, err := q.GetApplicationRequirementByID(ctx, reqID)
		if err != nil {
			return fmt.Errorf("requirement not found: %w", err)
		}

		app, err := q.GetApplicationByID(ctx, req.ApplicationID)
		if err != nil {
			return err
		}
		if err := ensureEditable(app); err != nil {
			return err
		}

		// Ensure the documents belong to this application's client
		for _, doc := range docs {
			if doc.ClientID != app.ClientID {
				return fmt.Errorf("vault document %d does not belong to this client", doc.ID)
			}
		}

		// Calculate the next version number
		latestVersion, err := q.GetLatestDocumentVersionNumber(ctx, reqID)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		nextVersion := int32(latestVersion + 1)

		// Create version record
		version, err = q.CreateApplicationDocumentVersion(ctx, tenant.CreateApplicationDocumentVersionParams{
			RequirementID:    reqID,
			VersionNumber:    nextVersion,
			UploadedByClient: isClient,
		})
		if err != nil {
			return err
		}

		// Link all vault files to the application requirement
		for _, doc := range docs {
			file, err := q.CreateRequirementVersionFile(ctx, tenant.CreateRequirementVersionFileParams{
				VersionID:    version.ID,
				FileUrl:      doc.StoragePath,
				DocumentName: doc.FileName,
			})
			if err != nil {
				return err
			}

			// Set the client_document_id on the file record
			err = q.UpdateVersionFileClientDocumentID(ctx, tenant.UpdateVersionFileClientDocumentIDParams{
				ID:               file.ID,
				ClientDocumentID: &doc.ID,
			})
			if err != nil {
				return err
			}
		}

		// Update requirement status
		status := "UPLOADED"
		if !isClient {
			status = "APPROVED" // if advocate uploads it, skip review phase
		}

		q.UpdateRequirementStatus(ctx, tenant.UpdateRequirementStatusParams{
			ID:              reqID,
			Status:          status,
			RejectionReason: nil,
		})

		userID := reqctx.MustUserID(ctx)
		var actorID *int64
		if userID > 0 {
			actorID = &userID
		}
		actorType := "CLIENT"
		if !isClient {
			actorType = "ORGANIZATION"
		}

		docNames := []string{}
		for _, doc := range docs {
			docNames = append(docNames, doc.FileName)
		}

		_, err = q.CreateTimelineEvent(ctx, tenant.CreateTimelineEventParams{
			ApplicationID: req.ApplicationID, EventType: "DOCUMENT_UPLOADED", ActorType: actorType, ActorID: actorID, Description: "Attached documents from vault: " + strings.Join(docNames, ", "), Metadata: []byte("{}"),
		})

		return nil
	})

	// If the attachment succeeded, we should grant access to this tenant if not already granted.
	if err == nil {
		for _, doc := range docs {
			conn, cerr := s.tm.Pub().GetClientConnectionByTenantAndClient(ctx, public.GetClientConnectionByTenantAndClientParams{
				TenantID: tenantID,
				ClientID: doc.ClientID,
			})
			if cerr == nil {
				// Ignore error if grant already exists
				_, _ = s.tm.Pub().CreateDocumentAccessGrant(ctx, public.CreateDocumentAccessGrantParams{
					DocumentID:   doc.ID,
					ConnectionID: conn.ID,
				})
			}
		}
	}

	return version, err
}

// UpdateApplicationStatus moves the overall application state.
func (s *Service) UpdateApplicationStatus(ctx context.Context, tenantID int64, appID int64, status string) (tenant.Application, error) {
	var app tenant.Application
	err := s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		var err error
		app, err = q.UpdateApplicationStatus(ctx, tenant.UpdateApplicationStatusParams{
			ID:     appID,
			Status: status,
		})
		return err
	})
	return app, err
}

// ReviewRequirement allows an advocate to approve or reject an uploaded document.
func (s *Service) ReviewRequirement(ctx context.Context, tenantID int64, reqID int64, status string, reason string) (tenant.ApplicationRequirement, error) {
	var req tenant.ApplicationRequirement
	err := s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		existingReq, err := q.GetApplicationRequirementByID(ctx, reqID)
		if err != nil {
			return err
		}

		app, err := q.GetApplicationByID(ctx, existingReq.ApplicationID)
		if err != nil {
			return err
		}
		if err := ensureEditable(app); err != nil {
			return err
		}

		var rejection *string
		if strings.ToUpper(status) == "REJECTED" && reason != "" {
			rejection = &reason
		}
		req, err = q.UpdateRequirementStatus(ctx, tenant.UpdateRequirementStatusParams{
			ID:              reqID,
			Status:          strings.ToUpper(status),
			RejectionReason: rejection,
		})

		if err == nil {
			userID := reqctx.MustUserID(ctx)
			var actorID *int64
			var reviewerName *string
			if userID > 0 {
				actorID = &userID
			}
			var commentStr *string
			if reason != "" {
				commentStr = &reason
			}

			q.CreateRequirementReview(ctx, tenant.CreateRequirementReviewParams{
				RequirementID: reqID,
				ReviewerID:    actorID,
				ReviewerType:  "ORGANIZATION",
				ReviewerName:  reviewerName,
				Status:        strings.ToUpper(status),
				Comment:       commentStr,
			})

			eventType := "DOCUMENT_APPROVED"
			desc := "Approved document: " + req.DocumentName
			if strings.ToUpper(status) == "REJECTED" {
				eventType = "DOCUMENT_REJECTED"
				desc = "Rejected document: " + req.DocumentName
			}
			q.CreateTimelineEvent(ctx, tenant.CreateTimelineEventParams{
				ApplicationID: req.ApplicationID, EventType: eventType, ActorType: "ORGANIZATION", ActorID: actorID, Description: desc, Metadata: []byte("{}"),
			})

			// Documents are now automatically vaulted at the time of upload,
			// so we no longer need a separate Phase 6 to vault them upon approval.
		}

		return err
	})
	return req, err
}

func (s *Service) saveToVault(ctx context.Context, tenantID int64, clientID int64, documentTypeID int64, documentName string, files []tenant.ApplicationDocumentVersionFile, q *tenant.Queries) {
	for _, file := range files {
		// If it's already linked to the vault (e.g. from a "Select From Vault" action),
		// we don't create a new document in the vault, but we MUST grant the organization access to it!
		if file.ClientDocumentID != nil {
			conn, err := s.tm.Pub().GetClientConnectionByTenantAndClient(ctx, public.GetClientConnectionByTenantAndClientParams{
				TenantID: tenantID,
				ClientID: clientID,
			})
			if err == nil {
				_, _ = s.tm.Pub().CreateDocumentAccessGrant(ctx, public.CreateDocumentAccessGrantParams{
					DocumentID:   *file.ClientDocumentID,
					ConnectionID: conn.ID,
				})
			}
			continue
		}

		// Attempt to read the actual file to get its size and calculate SHA-256
		var fileSize int64 = 0
		mimeType := "application/octet-stream"
		var sha256Hash *string

		if fileInfo, err := os.Stat(file.FileUrl); err == nil {
			fileSize = fileInfo.Size()
		}

		basename := filepath.Base(file.FileUrl)

		// Determine mime type from extension
		ext := strings.ToLower(filepath.Ext(basename))
		switch ext {
		case ".pdf":
			mimeType = "application/pdf"
		case ".jpg", ".jpeg":
			mimeType = "image/jpeg"
		case ".png":
			mimeType = "image/png"
		}

		// Ensure unique name based on original filename (file.DocumentName)
		baseName := file.DocumentName
		if baseName == "" {
			baseName = documentName
		}

		vaultFiles, _ := s.tm.Pub().ListClientVaultFiles(ctx, public.ListClientVaultFilesParams{
			ClientID:       clientID,
			DocumentTypeID: documentTypeID,
		})

		existingNames := make(map[string]bool)
		for _, vf := range vaultFiles {
			existingNames[vf.FileName] = true
		}

		finalName := baseName
		if existingNames[finalName] {
			ext2 := filepath.Ext(baseName)
			nameWithoutExt := strings.TrimSuffix(baseName, ext2)
			for i := 1; i < 1000; i++ {
				newName := fmt.Sprintf("%s (%d)%s", nameWithoutExt, i, ext2)
				if !existingNames[newName] {
					finalName = newName
					break
				}
			}
		}

		// Save to vault (creating a new entry)
		var actorID *int64
		if uID := reqctx.MustUserID(ctx); uID > 0 {
			actorID = &uID
		}

		doc, err := s.tm.Pub().CreateClientDocument(ctx, public.CreateClientDocumentParams{
			ClientID:         clientID,
			DocumentTypeID:   documentTypeID,
			FileName:         finalName,
			OriginalFileName: file.DocumentName, // Track original filename here as well
			FileSize:         fileSize,
			MimeType:         mimeType,
			Sha256Hash:       sha256Hash,
			StoragePath:      file.FileUrl, // Store reference to existing file on disk
			UploadedBy:       actorID,
			Source:           "APPROVED_APPLICATION",
		})

		if err != nil {
			fmt.Printf("ERROR saving to vault: %v\n", err)
		} else {
			// Update the tenant schema's application_document_version_files to link to the new vault document
			_ = q.UpdateVersionFileClientDocumentID(ctx, tenant.UpdateVersionFileClientDocumentIDParams{
				ID:               file.ID,
				ClientDocumentID: &doc.ID,
			})

			// Grant access to this organization
			// We need the connection ID between the client and this tenant.
			conn, err := s.tm.Pub().GetClientConnectionByTenantAndClient(ctx, public.GetClientConnectionByTenantAndClientParams{
				TenantID: tenantID,
				ClientID: clientID,
			})
			if err == nil {
				_, _ = s.tm.Pub().CreateDocumentAccessGrant(ctx, public.CreateDocumentAccessGrantParams{
					DocumentID:   doc.ID,
					ConnectionID: conn.ID,
				})
			}
		}
	}
}

// UploadFinalDeliverable uploads the final document, saves it to the client vault, and completes the application.
func (s *Service) UploadFinalDeliverable(ctx context.Context, tenantID int64, appID int64, file *multipart.FileHeader) (tenant.Application, error) {
	var app tenant.Application
	err := s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		// 1. Get the application to find the client_id
		existingApp, err := q.GetApplicationByID(ctx, appID)
		if err != nil {
			return fmt.Errorf("application not found: %w", err)
		}

		// 2. Save the file to disk
		ext := filepath.Ext(file.Filename)
		filename := fmt.Sprintf("final_%d_%s%s", appID, uuid.New().String()[:8], ext)
		uploadDir := filepath.Join("uploads", "clients", fmt.Sprintf("%d", existingApp.ClientID), "deliverables")

		if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
			return err
		}

		dstPath := filepath.Join(uploadDir, filename)
		dst, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer dst.Close()

		f, err := file.Open()
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := io.Copy(dst, f); err != nil {
			return err
		}

		// 3. Look up the document type "Deliverable" (or create it)
		docType, err := s.tm.Pub().GetDocumentTypeByName(ctx, "Deliverable")
		if err != nil {
			// fallback
			docType.ID = 1
		}

		// 3. Insert into public.client_documents
		doc, err := s.tm.Pub().CreateClientDocument(ctx, public.CreateClientDocumentParams{
			ClientID:       existingApp.ClientID,
			FileName:       existingApp.Title + " - " + file.Filename,
			DocumentTypeID: docType.ID,
			StoragePath:    dstPath,
		})
		if err != nil {
			return fmt.Errorf("failed to save to client vault: %w", err)
		}

		// 4. Update the application
		userID := reqctx.MustUserID(ctx)
		var actorID *int64
		if userID > 0 {
			actorID = &userID
		}

		_, err = q.AddApplicationDeliverable(ctx, tenant.AddApplicationDeliverableParams{
			ApplicationID:    appID,
			ClientDocumentID: doc.ID,
			UploadedByUserID: actorID,
		})
		if err != nil {
			return err
		}

		app, err = q.UpdateApplicationStatus(ctx, tenant.UpdateApplicationStatusParams{
			ID:     appID,
			Status: "COMPLETED",
		})

		q.CreateTimelineEvent(ctx, tenant.CreateTimelineEventParams{
			ApplicationID: app.ID, EventType: "FINAL_DOCUMENT_UPLOADED", ActorType: "ORGANIZATION", ActorID: actorID, Description: "Final deliverable uploaded", Metadata: []byte("{}"),
		})
		q.CreateTimelineEvent(ctx, tenant.CreateTimelineEventParams{
			ApplicationID: app.ID, EventType: "APPLICATION_COMPLETED", ActorType: "ORGANIZATION", ActorID: actorID, Description: "Application completed", Metadata: []byte("{}"),
		})

		return err
	})
	return app, err
}

// ensureEditable checks if the application can be edited based on its status.
func ensureEditable(app tenant.Application) error {
	if app.Status == "COMPLETED" {
		return fmt.Errorf("application is %s and cannot be modified", app.Status)
	}
	return nil
}

// UpdateApplication updates the application title.
func (s *Service) UpdateApplication(ctx context.Context, tenantID int64, appID int64, title string) (tenant.Application, error) {
	var app tenant.Application
	err := s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		existingApp, err := q.GetApplicationByID(ctx, appID)
		if err != nil {
			return err
		}

		if err := ensureEditable(existingApp); err != nil {
			return err
		}

		app, err = q.UpdateApplicationTitle(ctx, tenant.UpdateApplicationTitleParams{
			ID: appID, Title: title,
		})
		if err != nil {
			return err
		}

		userID := reqctx.MustUserID(ctx)
		var actorID *int64
		if userID > 0 {
			actorID = &userID
		}
		_, err = q.CreateTimelineEvent(ctx, tenant.CreateTimelineEventParams{
			ApplicationID: app.ID, EventType: "APPLICATION_EDITED", ActorType: "ORGANIZATION", ActorID: actorID, Description: "Application title updated", Metadata: []byte("{}"),
		})
		return err
	})
	return app, err
}

// ArchiveApplication archives the application.
func (s *Service) ArchiveApplication(ctx context.Context, tenantID int64, appID int64) error {
	return s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		userID := reqctx.MustUserID(ctx)
		var actorID *int64
		if userID > 0 {
			actorID = &userID
		}

		_, err := q.ArchiveApplication(ctx, tenant.ArchiveApplicationParams{
			ID: appID, ArchivedBy: actorID,
		})
		if err != nil {
			return err
		}

		_, err = q.CreateTimelineEvent(ctx, tenant.CreateTimelineEventParams{
			ApplicationID: appID, EventType: "APPLICATION_ARCHIVED", ActorType: "ORGANIZATION", ActorID: actorID, Description: "Application archived", Metadata: []byte("{}"),
		})
		return err
	})
}

// RestoreApplication restores the application from archive.
func (s *Service) RestoreApplication(ctx context.Context, tenantID int64, appID int64) error {
	return s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		_, err := q.RestoreApplication(ctx, appID)
		if err != nil {
			return err
		}

		userID := reqctx.MustUserID(ctx)
		var actorID *int64
		if userID > 0 {
			actorID = &userID
		}
		_, err = q.CreateTimelineEvent(ctx, tenant.CreateTimelineEventParams{
			ApplicationID: appID, EventType: "APPLICATION_RESTORED", ActorType: "ORGANIZATION", ActorID: actorID, Description: "Application restored from archive", Metadata: []byte("{}"),
		})
		return err
	})
}

// AddRequirement adds a new document requirement to the application.
func (s *Service) AddRequirement(ctx context.Context, tenantID int64, appID int64, req RequirementDto) (tenant.ApplicationRequirement, error) {
	var createdReq tenant.ApplicationRequirement
	err := s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		app, err := q.GetApplicationByID(ctx, appID)
		if err != nil {
			return err
		}
		if err := ensureEditable(app); err != nil {
			return err
		}

		docType, err := s.tm.Pub().GetDocumentType(ctx, req.DocumentTypeID)
		if err != nil {
			return fmt.Errorf("invalid document_type_id: %w", err)
		}

		var desc *string
		if req.Description != "" {
			d := req.Description
			desc = &d
		}

		createdReq, err = q.CreateApplicationRequirement(ctx, tenant.CreateApplicationRequirementParams{
			ApplicationID:  app.ID,
			DocumentTypeID: req.DocumentTypeID,
			DocumentName:   docType.Name,
			Description:    desc,
			DisplayOrder:   req.DisplayOrder,
			IsRequired:     req.IsRequired,
		})
		if err != nil {
			return err
		}

		userID := reqctx.MustUserID(ctx)
		var actorID *int64
		if userID > 0 {
			actorID = &userID
		}
		_, err = q.CreateTimelineEvent(ctx, tenant.CreateTimelineEventParams{
			ApplicationID: app.ID, EventType: "REQUIREMENT_ADDED", ActorType: "ORGANIZATION", ActorID: actorID, Description: "Added new requirement: " + docType.Name, Metadata: []byte("{}"),
		})
		return err
	})
	return createdReq, err
}

type TimelineEventDTO struct {
	tenant.ApplicationTimeline
	ActorName string
}

// GetTimeline fetches the timeline for an application.
func (s *Service) GetTimeline(ctx context.Context, tenantID int64, appID int64) ([]TimelineEventDTO, error) {
	var timeline []tenant.ApplicationTimeline
	err := s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		var err error
		timeline, err = q.ListTimelineEvents(ctx, appID)
		return err
	})
	if err != nil {
		return nil, err
	}

	var dtos []TimelineEventDTO
	for _, event := range timeline {
		dto := TimelineEventDTO{ApplicationTimeline: event}
		if event.ActorID != nil {
			var firstName, lastName string
			err := s.tm.Pool().QueryRow(ctx, "SELECT first_name, last_name FROM public.user_profiles WHERE user_id = $1", *event.ActorID).Scan(&firstName, &lastName)
			if err == nil {
				dto.ActorName = strings.TrimSpace(firstName + " " + lastName)
			}
			
			// If not found in user_profiles or name is empty, fall back to email
			if dto.ActorName == "" {
				var email string
				err = s.tm.Pool().QueryRow(ctx, "SELECT email FROM public.users WHERE id = $1", *event.ActorID).Scan(&email)
				if err == nil {
					dto.ActorName = email
				}
			}
		}
		dtos = append(dtos, dto)
	}
	return dtos, nil
}

// GetDocumentVersionFile returns the file path and download filename for a given file.
func (s *Service) GetDocumentVersionFile(ctx context.Context, tenantID, fileID int64) (string, string, error) {
	var filePath, filename string
	err := s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		file, err := q.GetVersionFileByID(ctx, fileID)
		if err != nil {
			return err
		}

		if file.ClientDocumentID != nil {
			// Enforce access control
			// 1. Get version -> req -> app -> client
			version, err := q.GetDocumentVersionByID(ctx, file.VersionID)
			if err != nil {
				return err
			}
			req, err := q.GetApplicationRequirementByID(ctx, version.RequirementID)
			if err != nil {
				return err
			}
			app, err := q.GetApplicationByID(ctx, req.ApplicationID)
			if err != nil {
				return err
			}

			// Only enforce access control if the requester is NOT the client themselves
			if reqctx.TenantID(ctx) != 0 {
				// 2. Get connection
				conn, err := s.tm.Pub().GetClientConnectionByTenantAndClient(ctx, public.GetClientConnectionByTenantAndClientParams{
					TenantID: tenantID,
					ClientID: app.ClientID,
				})
				if err != nil {
					return err
				}

				// 3. Check access grant
				_, err = s.tm.Pub().CheckDocumentAccess(ctx, public.CheckDocumentAccessParams{
					DocumentID:   *file.ClientDocumentID,
					ConnectionID: conn.ID,
				})
				if err != nil {
					return errors.New("access to this vault document has been revoked by the client")
				}
			}
		}

		filePath = file.FileUrl
		filename = file.DocumentName
		return nil
	})
	return filePath, filename, err
}

// GetFinalDeliverableFile returns the file path and download filename for a specific final deliverable.
func (s *Service) GetFinalDeliverableFile(ctx context.Context, tenantID, appID, deliverableID int64) (string, string, error) {
	var filePath, filename string
	err := s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		// Verify app exists and belongs to tenant
		app, err := q.GetApplicationByID(ctx, appID)
		if err != nil {
			return err
		}
		
		// Enforce access control for clients
		id, _ := reqctx.Get(ctx)
		if id.Role == "Client" {
			userID := reqctx.MustUserID(ctx)
			profile, err := s.tm.Pub().GetClientProfileByUserId(ctx, userID)
			if err != nil {
				return fmt.Errorf("client profile not found: %w", err)
			}
			if app.ClientID != profile.ID {
				return fmt.Errorf("access denied to application")
			}
		}

		deliverable, err := q.GetApplicationDeliverableByID(ctx, deliverableID)
		if err != nil {
			return err
		}
		
		if deliverable.ApplicationID != appID {
			return fmt.Errorf("deliverable does not belong to this application")
		}

		doc, err := s.tm.Pub().GetClientDocument(ctx, deliverable.ClientDocumentID)
		if err != nil {
			return err
		}

		filePath = doc.StoragePath
		filename = doc.FileName + filepath.Ext(filePath)
		return nil
	})
	return filePath, filename, err
}

// LogTimelineEvent creates a generic timeline event for the application.
func (s *Service) LogTimelineEvent(ctx context.Context, tenantID int64, appID int64, eventType string, actorType string, description string) error {
	return s.tm.InTenantTxByID(ctx, tenantID, func(q *tenant.Queries) error {
		userID := reqctx.MustUserID(ctx)
		var actorID *int64
		if userID > 0 {
			actorID = &userID
		}

		_, err := q.CreateTimelineEvent(ctx, tenant.CreateTimelineEventParams{
			ApplicationID: appID, EventType: eventType, ActorType: actorType, ActorID: actorID, Description: description, Metadata: []byte("{}"),
		})
		return err
	})
}
