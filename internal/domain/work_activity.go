package domain

import (
	"context"
	"time"
)

type WorkActivity struct {
	ID        uint      `json:"id"`
	WorkType  string    `json:"work_type"`
	WorkID    uint      `json:"work_id"`
	Kind      string    `json:"kind"`
	Message   string    `json:"message"`
	ActorID   uint      `json:"-"`
	ActorName string    `json:"actor_name"`
	ActorRole string    `json:"actor_role"`
	CreatedAt time.Time `json:"created_at"`
}

type WorkActivityRepository interface {
	Migrate() error
	Create(ctx context.Context, activity *WorkActivity) error
	List(ctx context.Context, workType string, workID uint, limit int) ([]WorkActivity, error)
}
