package postgres

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ticket-system/internal/domain"
)

func TestTicketRepositoryPersistsPausedSLAAndAdjustedDueTime(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tickets.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	repository := NewTicketRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	due := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Second)
	ticket := domain.Ticket{EmployeeName: "Test User", Problem: "VPN is disconnected", AssetTag: "AST-000123", CreatedBy: "Test Agent", Priority: "medium", SLADueAt: due, Status: domain.StatusPending}
	if err := repository.Create(ctx, &ticket); err != nil {
		t.Fatal(err)
	}
	pausedAt := time.Now().UTC().Truncate(time.Second)
	if ok, err := repository.SetSLA(ctx, ticket.ID, &pausedAt, due); err != nil || !ok {
		t.Fatalf("pause SLA = %v, %v", ok, err)
	}
	paused, err := repository.GetByKey(ctx, ticket.TicketNumber)
	if err != nil || paused.SLAPausedAt == nil || !paused.SLAPausedAt.Equal(pausedAt) || paused.AssetTag != ticket.AssetTag || paused.CreatedBy != ticket.CreatedBy {
		t.Fatalf("paused ticket = %#v, %v", paused, err)
	}
	resumedDue := due.Add(2 * time.Hour)
	if ok, err := repository.SetSLA(ctx, ticket.ID, nil, resumedDue); err != nil || !ok {
		t.Fatalf("resume SLA = %v, %v", ok, err)
	}
	resumed, err := repository.GetByKey(ctx, ticket.TicketNumber)
	if err != nil || resumed.SLAPausedAt != nil || !resumed.SLADueAt.Equal(resumedDue) {
		t.Fatalf("resumed ticket = %#v, %v", resumed, err)
	}
}
