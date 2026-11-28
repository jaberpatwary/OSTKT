package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"ticket-system/internal/domain"
)

var (
	ErrInvalidWorkType = errors.New("work type must be ticket or task")
	ErrInvalidActivity = errors.New("invalid activity entry")
)

type WorkActivityService struct{ repository domain.WorkActivityRepository }

func NewWorkActivityService(repository domain.WorkActivityRepository) *WorkActivityService {
	return &WorkActivityService{repository: repository}
}

func (s *WorkActivityService) Record(ctx context.Context, actor domain.User, workType string, workID uint, kind, message string) (domain.WorkActivity, error) {
	workType = strings.ToLower(strings.TrimSpace(workType))
	if workType != "ticket" && workType != "task" {
		return domain.WorkActivity{}, ErrInvalidWorkType
	}
	if workID == 0 {
		return domain.WorkActivity{}, ErrInvalidActivity
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "comment", "created", "status_changed", "assignee_changed", "priority_changed", "updated", "attachment_added":
	default:
		return domain.WorkActivity{}, ErrInvalidActivity
	}
	message = strings.TrimSpace(message)
	if message == "" || len([]rune(message)) > 2000 {
		return domain.WorkActivity{}, ErrInvalidActivity
	}
	actorName := strings.TrimSpace(actor.DisplayName)
	if actorName == "" {
		actorName = strings.TrimSpace(actor.Username)
	}
	if actor.ID == 0 || actorName == "" || actor.Role == "" {
		return domain.WorkActivity{}, ErrInvalidActivity
	}
	activity := domain.WorkActivity{
		WorkType: workType, WorkID: workID, Kind: kind, Message: message,
		ActorID: actor.ID, ActorName: actorName, ActorRole: actor.Role,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.repository.Create(ctx, &activity); err != nil {
		return domain.WorkActivity{}, err
	}
	return activity, nil
}

func (s *WorkActivityService) List(ctx context.Context, workType string, workID uint, limit int) ([]domain.WorkActivity, error) {
	workType = strings.ToLower(strings.TrimSpace(workType))
	if workType != "ticket" && workType != "task" {
		return nil, ErrInvalidWorkType
	}
	if workID == 0 {
		return nil, ErrInvalidActivity
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	return s.repository.List(ctx, workType, workID, limit)
}
