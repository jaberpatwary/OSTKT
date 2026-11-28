package domain

import (
	"context"
	"time"
)

type Notification struct {
	ID        uint       `json:"id"`
	Kind      string     `json:"kind"`
	Severity  string     `json:"severity"`
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	Link      string     `json:"link"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type NotificationInput struct {
	EventKey string
	Kind     string
	Severity string
	Title    string
	Message  string
	Link     string
}

type NotificationRepository interface {
	Migrate() error
	CreateForUsers(ctx context.Context, userIDs []uint, input NotificationInput) error
	ListForUser(ctx context.Context, userID uint, unreadOnly bool, limit int) ([]Notification, error)
	CountUnread(ctx context.Context, userID uint) (int64, error)
	MarkRead(ctx context.Context, userID, notificationID uint) (bool, error)
	MarkAllRead(ctx context.Context, userID uint) error
}
