package domain

import (
	"context"
	"errors"
	"time"
)

const (
	StatusDone       = "done"
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
)

var (
	ErrNotFound     = errors.New("record not found")
	ErrDuplicateKey = errors.New("duplicate key")
)

type Employee struct {
	Name       string
	EmployeeID string
	Department string
	Email      string
	Phone      string
}

type Ticket struct {
	ID           uint
	TicketNumber string
	EmployeeName string
	EmployeeID   string
	Department   string
	Problem      string
	AssignedTo   string
	Priority     string
	SLADueAt     time.Time
	Email        string
	Phone        string
	Status       string
	EmailSentAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (t Ticket) Employee() Employee {
	return Employee{
		Name: t.EmployeeName, EmployeeID: t.EmployeeID, Department: t.Department,
		Email: t.Email, Phone: t.Phone,
	}
}

type TicketFilter struct {
	Status       string
	CreatedFrom  time.Time
	CreatedUntil time.Time
	Limit        int
}

type Task struct {
	ID          uint
	Title       string
	Description string
	CreatedBy   string
	AssignedTo  string
	Priority    string
	DueDate     string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type TaskFilter struct {
	Status   string
	Assignee string
	Query    string
	Limit    int
}

type TaskRepository interface {
	Create(ctx context.Context, task *Task) error
	Get(ctx context.Context, id uint) (Task, error)
	List(ctx context.Context, filter TaskFilter) ([]Task, int64, error)
	Update(ctx context.Context, id uint, changes map[string]any) (bool, error)
}

type TicketRepository interface {
	Create(ctx context.Context, ticket *Ticket) error
	GetByKey(ctx context.Context, key string) (Ticket, error)
	CompletePending(ctx context.Context, id uint) (bool, error)
	SetStatus(ctx context.Context, id uint, status string) (bool, error)
	SetAssignee(ctx context.Context, id uint, assignee string) (bool, error)
	SetPriority(ctx context.Context, id uint, priority string, slaDueAt time.Time) (bool, error)
	SetEmailSentAt(ctx context.Context, id uint, sentAt time.Time) error
	List(ctx context.Context, filter TicketFilter) ([]Ticket, int64, error)
}
