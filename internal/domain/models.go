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
	ErrNotFound          = errors.New("record not found")
	ErrDuplicateKey      = errors.New("duplicate key")
	ErrLastAdmin         = errors.New("cannot deactivate or demote the last active administrator")
	ErrInvalidTransition = errors.New("invalid state transition")
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
	AssetTag     string
	CreatedBy    string
	AssignedTo   string
	Priority     string
	SLADueAt     time.Time
	SLAPausedAt  *time.Time
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
	Query        string
	Limit        int
	Offset       int
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
	Status       string
	Assignee     string
	Query        string
	CreatedFrom  time.Time
	CreatedUntil time.Time
	Limit        int
	Offset       int
}

// User is an internal application account. Password material is deliberately
// kept out of the domain model returned to HTTP clients.
type User struct {
	ID          uint      `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	Active      bool      `json:"active"`
	MFAEnabled  bool      `json:"mfa_enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UserInput struct {
	Username    string
	DisplayName string
	Role        string
	Password    string
	Active      bool
}

type UserUpdate struct {
	DisplayName string
	Role        string
	Password    string
	Active      bool
}

type AuditEvent struct {
	ID         uint      `json:"id"`
	UserID     uint      `json:"user_id"`
	Username   string    `json:"username"`
	Role       string    `json:"role"`
	Action     string    `json:"action"`
	Resource   string    `json:"resource"`
	StatusCode int       `json:"status_code"`
	IPAddress  string    `json:"ip_address"`
	UserAgent  string    `json:"user_agent"`
	CreatedAt  time.Time `json:"created_at"`
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
	SetSLA(ctx context.Context, id uint, pausedAt *time.Time, slaDueAt time.Time) (bool, error)
	SetEmailSentAt(ctx context.Context, id uint, sentAt time.Time) error
	List(ctx context.Context, filter TicketFilter) ([]Ticket, int64, error)
}
