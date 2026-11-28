package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"

	"ticket-system/internal/domain"
)

type workActivityRecord struct {
	ID        uint      `gorm:"primaryKey"`
	WorkType  string    `gorm:"size:12;not null;index:idx_work_activity_order,priority:1"`
	WorkID    uint      `gorm:"not null;index:idx_work_activity_order,priority:2"`
	Kind      string    `gorm:"size:32;not null;index"`
	Message   string    `gorm:"size:2000;not null"`
	ActorID   uint      `gorm:"index"`
	ActorName string    `gorm:"size:120;not null"`
	ActorRole string    `gorm:"size:20;not null"`
	CreatedAt time.Time `gorm:"index:idx_work_activity_order,priority:3"`
}

func (workActivityRecord) TableName() string { return "work_activities" }

func (r workActivityRecord) toDomain() domain.WorkActivity {
	return domain.WorkActivity{
		ID: r.ID, WorkType: r.WorkType, WorkID: r.WorkID,
		Kind: r.Kind, Message: r.Message, ActorID: r.ActorID, ActorName: r.ActorName,
		ActorRole: r.ActorRole, CreatedAt: r.CreatedAt,
	}
}

type WorkActivityRepository struct{ db *gorm.DB }

func NewWorkActivityRepository(db *gorm.DB) *WorkActivityRepository {
	return &WorkActivityRepository{db: db}
}

func (r *WorkActivityRepository) Migrate() error {
	return r.db.AutoMigrate(&workActivityRecord{})
}

func (r *WorkActivityRepository) Create(ctx context.Context, activity *domain.WorkActivity) error {
	record := workActivityRecord{
		WorkType: activity.WorkType, WorkID: activity.WorkID,
		Kind: activity.Kind, Message: activity.Message,
		ActorID: activity.ActorID, ActorName: activity.ActorName,
		ActorRole: activity.ActorRole, CreatedAt: activity.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return err
	}
	*activity = record.toDomain()
	return nil
}

func (r *WorkActivityRepository) List(ctx context.Context, workType string, workID uint, limit int) ([]domain.WorkActivity, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	records := make([]workActivityRecord, 0)
	if err := r.db.WithContext(ctx).Where("work_type = ? AND work_id = ?", workType, workID).
		Order("created_at DESC, id DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	activities := make([]domain.WorkActivity, 0, len(records))
	for i := len(records) - 1; i >= 0; i-- {
		activities = append(activities, records[i].toDomain())
	}
	return activities, nil
}
