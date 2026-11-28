package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"ticket-system/internal/domain"
)

const (
	MaxTaskLimit        = 10000
	DefaultTaskCreator  = "Md. Tanvir Ahamed Sarkar"
	DefaultTaskAssignee = "IT Team"
)

var (
	ErrTaskTitleRequired      = errors.New("task title is required")
	ErrTaskTitleTooLong       = errors.New("task title must be 160 characters or fewer")
	ErrTaskDescriptionTooLong = errors.New("task description must be 2000 characters or fewer")
	ErrTaskCreatorRequired    = errors.New("person who gave the task is required")
	ErrTaskCreatorTooLong     = errors.New("person who gave the task must be 100 characters or fewer")
	ErrTaskAssigneeTooLong    = errors.New("task assignee must be 100 characters or fewer")
	ErrTaskInvalidPriority    = errors.New("priority must be low, medium, high, or urgent")
	ErrTaskInvalidStatus      = errors.New("status must be pending, in_progress, or done")
	ErrTaskInvalidDueDate     = errors.New("due date must use YYYY-MM-DD format")
	ErrTaskNoChanges          = errors.New("at least one task field must be updated")
)

type CreateTaskInput struct {
	Title       string
	Description string
	CreatedBy   string
	AssignedTo  string
	Priority    string
	DueDate     string
}

type UpdateTaskInput struct {
	Title       *string
	Description *string
	CreatedBy   *string
	AssignedTo  *string
	Priority    *string
	DueDate     *string
	Status      *string
}

type TaskService struct {
	repository domain.TaskRepository
}

func NewTaskService(repository domain.TaskRepository) *TaskService {
	return &TaskService{repository: repository}
}

func (s *TaskService) Create(ctx context.Context, input CreateTaskInput) (domain.Task, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return domain.Task{}, ErrTaskTitleRequired
	}
	if len([]rune(title)) > 160 {
		return domain.Task{}, ErrTaskTitleTooLong
	}
	description := strings.TrimSpace(input.Description)
	if len([]rune(description)) > 2000 {
		return domain.Task{}, ErrTaskDescriptionTooLong
	}
	createdBy := strings.TrimSpace(input.CreatedBy)
	if createdBy == "" {
		createdBy = DefaultTaskCreator
	}
	if len([]rune(createdBy)) > 100 {
		return domain.Task{}, ErrTaskCreatorTooLong
	}
	assignee := strings.TrimSpace(input.AssignedTo)
	if assignee == "" {
		assignee = DefaultTaskAssignee
	}
	if len([]rune(assignee)) > 100 {
		return domain.Task{}, ErrTaskAssigneeTooLong
	}
	priority := normalizeTaskPriority(input.Priority)
	if !validTaskPriority(priority) {
		return domain.Task{}, ErrTaskInvalidPriority
	}
	dueDate := strings.TrimSpace(input.DueDate)
	if !validTaskDueDate(dueDate) {
		return domain.Task{}, ErrTaskInvalidDueDate
	}
	task := domain.Task{Title: title, Description: description, CreatedBy: createdBy, AssignedTo: assignee, Priority: priority, DueDate: dueDate, Status: domain.StatusPending}
	if err := s.repository.Create(ctx, &task); err != nil {
		return domain.Task{}, err
	}
	return task, nil
}

func (s *TaskService) List(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, int64, error) {
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	if filter.Status != "" && !validTaskStatus(filter.Status) {
		return nil, 0, ErrTaskInvalidStatus
	}
	filter.Assignee = strings.TrimSpace(filter.Assignee)
	filter.Query = strings.TrimSpace(filter.Query)
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if filter.Offset > 1000000 {
		filter.Offset = 1000000
	}
	if filter.Limit <= 0 || filter.Limit > MaxTaskLimit {
		filter.Limit = 100
	}
	return s.repository.List(ctx, filter)
}

func (s *TaskService) Update(ctx context.Context, id uint, input UpdateTaskInput) (domain.Task, error) {
	changes := make(map[string]any)
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" {
			return domain.Task{}, ErrTaskTitleRequired
		}
		if len([]rune(title)) > 160 {
			return domain.Task{}, ErrTaskTitleTooLong
		}
		changes["title"] = title
	}
	if input.Description != nil {
		description := strings.TrimSpace(*input.Description)
		if len([]rune(description)) > 2000 {
			return domain.Task{}, ErrTaskDescriptionTooLong
		}
		changes["description"] = description
	}
	if input.CreatedBy != nil {
		createdBy := strings.TrimSpace(*input.CreatedBy)
		if createdBy == "" {
			createdBy = DefaultTaskCreator
		}
		if len([]rune(createdBy)) > 100 {
			return domain.Task{}, ErrTaskCreatorTooLong
		}
		changes["created_by"] = createdBy
	}
	if input.AssignedTo != nil {
		assignee := strings.TrimSpace(*input.AssignedTo)
		if assignee == "" {
			assignee = DefaultTaskAssignee
		}
		if len([]rune(assignee)) > 100 {
			return domain.Task{}, ErrTaskAssigneeTooLong
		}
		changes["assigned_to"] = assignee
	}
	if input.Priority != nil {
		priority := normalizeTaskPriority(*input.Priority)
		if !validTaskPriority(priority) {
			return domain.Task{}, ErrTaskInvalidPriority
		}
		changes["priority"] = priority
	}
	if input.DueDate != nil {
		dueDate := strings.TrimSpace(*input.DueDate)
		if !validTaskDueDate(dueDate) {
			return domain.Task{}, ErrTaskInvalidDueDate
		}
		changes["due_date"] = dueDate
	}
	if input.Status != nil {
		status := strings.ToLower(strings.TrimSpace(*input.Status))
		if !validTaskStatus(status) {
			return domain.Task{}, ErrTaskInvalidStatus
		}
		changes["status"] = status
	}
	if len(changes) == 0 {
		return domain.Task{}, ErrTaskNoChanges
	}
	updated, err := s.repository.Update(ctx, id, changes)
	if err != nil {
		return domain.Task{}, err
	}
	if !updated {
		return domain.Task{}, domain.ErrNotFound
	}
	return s.repository.Get(ctx, id)
}

func normalizeTaskPriority(priority string) string {
	priority = strings.ToLower(strings.TrimSpace(priority))
	if priority == "" {
		return "medium"
	}
	return priority
}

func validTaskPriority(priority string) bool {
	switch priority {
	case "low", "medium", "high", "urgent":
		return true
	default:
		return false
	}
}

func validTaskStatus(status string) bool {
	switch status {
	case domain.StatusPending, domain.StatusInProgress, domain.StatusDone:
		return true
	default:
		return false
	}
}

func validTaskDueDate(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}
