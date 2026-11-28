package postgres

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ticket-system/internal/domain"
)

func newTestPlatformRepository(t *testing.T) *PlatformRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "platform.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	repository := NewPlatformRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return repository
}

func TestPlatformRepositoryPersistsWorkspaceRecordsAndEnforcesChangeFlow(t *testing.T) {
	ctx := context.Background()
	repository := newTestPlatformRepository(t)

	service, err := repository.SaveService(ctx, domain.ServiceCatalogItem{Name: "New laptop", Category: "Workplace", Description: "Laptop setup and required tools", Priority: "high", Active: true})
	if err != nil {
		t.Fatalf("SaveService() error = %v", err)
	}
	services, err := repository.ListServices(ctx, false)
	foundService := false
	for _, item := range services {
		if item.ID == service.ID {
			foundService = true
		}
	}
	if err != nil || !foundService {
		t.Fatalf("ListServices() = %#v, %v", services, err)
	}
	service.Description, service.Active = "Updated description", false
	service, err = repository.SaveService(ctx, service)
	if err != nil || service.Description != "Updated description" || service.CreatedAt.IsZero() {
		t.Fatalf("update service = %#v, %v", service, err)
	}

	article, err := repository.SaveArticle(ctx, domain.KnowledgeArticle{Title: "Reset VPN", Body: "Check the connection, then sign in again.", Tags: "vpn,network", Published: true})
	if err != nil {
		t.Fatalf("SaveArticle() error = %v", err)
	}
	articles, err := repository.ListArticles(ctx, "network", false)
	if err != nil || len(articles) != 1 || articles[0].ID != article.ID {
		t.Fatalf("ListArticles() = %#v, %v", articles, err)
	}
	drafts, err := repository.ListArticles(ctx, "", true)
	if err != nil || len(drafts) != 1 {
		t.Fatalf("ListArticles(include drafts) = %#v, %v", drafts, err)
	}
	article.Body, article.Published = "Updated steps", false
	article, err = repository.SaveArticle(ctx, article)
	if err != nil || article.Body != "Updated steps" || article.Published || article.CreatedAt.IsZero() {
		t.Fatalf("update article = %#v, %v", article, err)
	}

	asset, err := repository.SaveAsset(ctx, domain.Asset{AssetTag: "AST-0001", Name: "ThinkPad", Category: "Laptop", Status: "assigned", AssignedEmployeeName: "Tajdin Bhai", AssignedEmployeeID: "2916"})
	if err != nil {
		t.Fatalf("SaveAsset() error = %v", err)
	}
	assets, err := repository.ListAssets(ctx, "2916")
	if err != nil || len(assets) != 1 || assets[0].AssetTag != asset.AssetTag {
		t.Fatalf("ListAssets() = %#v, %v", assets, err)
	}
	asset.Notes = "Warranty checked"
	asset, err = repository.SaveAsset(ctx, asset)
	if err != nil || asset.Notes != "Warranty checked" || asset.CreatedAt.IsZero() {
		t.Fatalf("update asset = %#v, %v", asset, err)
	}

	change, err := repository.SaveChange(ctx, domain.ChangeRequest{Title: "Core switch upgrade", RequestedBy: "Service desk", Risk: "high", Status: "requested"})
	if err != nil {
		t.Fatalf("SaveChange() error = %v", err)
	}
	if _, err = repository.SetChangeStatus(ctx, change.ID, "completed"); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("SetChangeStatus(requested → completed) error = %v", err)
	}
	if _, err = repository.SetChangeStatus(ctx, change.ID, "approved"); err != nil {
		t.Fatalf("approve change: %v", err)
	}
	if _, err = repository.SetChangeStatus(ctx, change.ID, "scheduled"); err != nil {
		t.Fatalf("schedule change: %v", err)
	}
	completed, err := repository.SetChangeStatus(ctx, change.ID, "completed")
	if err != nil || completed.Status != "completed" {
		t.Fatalf("complete change = %#v, %v", completed, err)
	}

	incident, err := repository.SaveIncident(ctx, domain.IncidentRecord{RecordType: "incident", Title: "Office network outage", Impact: "department", Status: "open", RelatedTickets: "TKT-000111"})
	if err != nil {
		t.Fatalf("SaveIncident() error = %v", err)
	}
	if _, err := repository.SetIncidentStatus(ctx, incident.ID, "closed"); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("invalid incident transition error = %v", err)
	}
	if _, err := repository.SetIncidentStatus(ctx, incident.ID, "investigating"); err != nil {
		t.Fatalf("start incident investigation: %v", err)
	}
	if _, err := repository.SetIncidentStatus(ctx, incident.ID, "identified"); err != nil {
		t.Fatalf("identify root cause: %v", err)
	}
	if _, err := repository.SetIncidentStatus(ctx, incident.ID, "monitoring"); err != nil {
		t.Fatalf("monitor recovery: %v", err)
	}
	resolved, err := repository.SetIncidentStatus(ctx, incident.ID, "resolved")
	if err != nil || resolved.ResolvedAt == nil {
		t.Fatalf("resolve incident = %#v, %v", resolved, err)
	}
	if _, err := repository.SetIncidentStatus(ctx, incident.ID, "investigating"); err != nil {
		t.Fatalf("reopen incident: %v", err)
	}
	incidents, err := repository.ListIncidents(ctx, "incident", "investigating")
	if err != nil || len(incidents) != 1 || incidents[0].ResolvedAt != nil {
		t.Fatalf("ListIncidents() = %#v, %v", incidents, err)
	}
}

func TestInboundEmailClaimIsPersistentAndRetryableAfterFailure(t *testing.T) {
	ctx := context.Background()
	repository := newTestPlatformRepository(t)
	claimed, err := repository.ClaimInboundEmail(ctx, "provider-123@example.test")
	if err != nil || !claimed {
		t.Fatalf("first ClaimInboundEmail() = %v, %v", claimed, err)
	}
	claimed, err = repository.ClaimInboundEmail(ctx, "provider-123@example.test")
	if err != nil || claimed {
		t.Fatalf("duplicate ClaimInboundEmail() = %v, %v", claimed, err)
	}
	if err := repository.ReleaseInboundEmail(ctx, "provider-123@example.test"); err != nil {
		t.Fatal(err)
	}
	claimed, err = repository.ClaimInboundEmail(ctx, "provider-123@example.test")
	if err != nil || !claimed {
		t.Fatalf("claim after release = %v, %v", claimed, err)
	}
	if err := repository.CompleteInboundEmail(ctx, "provider-123@example.test", "TKT-000123"); err != nil {
		t.Fatal(err)
	}
	_, completed, err := repository.InboundEmailStatus(ctx, "provider-123@example.test")
	if err != nil || !completed {
		t.Fatalf("InboundEmailStatus() completed = %v, %v", completed, err)
	}
	claimed, err = repository.ClaimInboundEmail(ctx, "provider-123@example.test")
	if err != nil || claimed {
		t.Fatalf("completed message should not be claimed again: %v, %v", claimed, err)
	}
}
