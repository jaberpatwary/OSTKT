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

func TestTaskRepositoryFiltersByCreatedDateRange(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tasks.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	repository := NewTaskRepository(db)
	if err := repository.Migrate(); err != nil {
		t.Fatal(err)
	}
	dayStart := time.Date(2026, time.November, 28, 0, 0, 0, 0, time.FixedZone("BDT", 6*60*60)).UTC()
	records := []taskRecord{
		{Title: "Yesterday task", Status: domain.StatusPending, CreatedAt: dayStart.Add(-time.Minute)},
		{Title: "Today task", Status: domain.StatusPending, CreatedAt: dayStart.Add(time.Hour)},
		{Title: "Tomorrow task", Status: domain.StatusPending, CreatedAt: dayStart.Add(24 * time.Hour)},
	}
	for _, record := range records {
		if err := db.Create(&record).Error; err != nil {
			t.Fatal(err)
		}
	}

	tasks, total, err := repository.List(context.Background(), domain.TaskFilter{
		CreatedFrom: dayStart, CreatedUntil: dayStart.Add(24 * time.Hour), Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(tasks) != 1 || tasks[0].Title != "Today task" {
		t.Fatalf("date-filtered tasks = %#v (total %d), want only today's task", tasks, total)
	}
}
