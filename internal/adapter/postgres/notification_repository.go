package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ticket-system/internal/domain"
)

type notificationRecord struct {
	ID        uint       `gorm:"primaryKey"`
	UserID    uint       `gorm:"not null;uniqueIndex:idx_notifications_user_event;index:idx_notifications_user_created,priority:1"`
	EventKey  string     `gorm:"size:180;not null;uniqueIndex:idx_notifications_user_event"`
	Kind      string     `gorm:"size:32;not null;index"`
	Severity  string     `gorm:"size:16;not null"`
	Title     string     `gorm:"size:180;not null"`
	Message   string     `gorm:"size:2000;not null"`
	Link      string     `gorm:"size:240;not null"`
	ReadAt    *time.Time `gorm:"index:idx_notifications_user_created,priority:2"`
	CreatedAt time.Time  `gorm:"index:idx_notifications_user_created,priority:3"`
}

func (notificationRecord) TableName() string { return "notifications" }

func (r notificationRecord) toDomain() domain.Notification {
	return domain.Notification{
		ID: r.ID, Kind: r.Kind, Severity: r.Severity, Title: r.Title,
		Message: r.Message, Link: r.Link, ReadAt: r.ReadAt, CreatedAt: r.CreatedAt,
	}
}

type NotificationRepository struct{ db *gorm.DB }

func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) Migrate() error {
	return r.db.AutoMigrate(&notificationRecord{})
}

func (r *NotificationRepository) CreateForUsers(ctx context.Context, userIDs []uint, input domain.NotificationInput) error {
	if len(userIDs) == 0 {
		return nil
	}
	seen := make(map[uint]struct{}, len(userIDs))
	records := make([]notificationRecord, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID == 0 {
			continue
		}
		if _, exists := seen[userID]; exists {
			continue
		}
		seen[userID] = struct{}{}
		records = append(records, notificationRecord{
			UserID: userID, EventKey: input.EventKey, Kind: input.Kind,
			Severity: input.Severity, Title: input.Title,
			Message: input.Message, Link: input.Link,
		})
	}
	if len(records) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&records, 50).Error
}

func (r *NotificationRepository) ListForUser(ctx context.Context, userID uint, unreadOnly bool, limit int) ([]domain.Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	query := r.db.WithContext(ctx).Model(&notificationRecord{}).Where("user_id = ?", userID)
	if unreadOnly {
		query = query.Where("read_at IS NULL")
	}
	records := make([]notificationRecord, 0)
	if err := query.Order("created_at DESC, id DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	notifications := make([]domain.Notification, 0, len(records))
	for _, record := range records {
		notifications = append(notifications, record.toDomain())
	}
	return notifications, nil
}

func (r *NotificationRepository) CountUnread(ctx context.Context, userID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&notificationRecord{}).Where("user_id = ? AND read_at IS NULL", userID).Count(&count).Error
	return count, err
}

func (r *NotificationRepository) MarkRead(ctx context.Context, userID, notificationID uint) (bool, error) {
	readAt := time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&notificationRecord{}).
		Where("id = ? AND user_id = ? AND read_at IS NULL", notificationID, userID).
		Update("read_at", readAt)
	return result.RowsAffected > 0, result.Error
}

func (r *NotificationRepository) MarkAllRead(ctx context.Context, userID uint) error {
	return r.db.WithContext(ctx).Model(&notificationRecord{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Update("read_at", time.Now().UTC()).Error
}
