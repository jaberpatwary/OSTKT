package domain

import (
	"context"
	"errors"
	"time"
)

const (
	StatusDone    = "done"
	StatusPending = "pending"
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
	Email        string
	Phone        string
	Status       string
	EmailSentAt  *time.Time
	CreatedAt    time.Time
}

func (t Ticket) Employee() Employee {
	return Employee{
		Name: t.EmployeeName, EmployeeID: t.EmployeeID, Department: t.Department,
		Email: t.Email, Phone: t.Phone,
	}
}

type TicketFilter struct {
	Status string
	Limit  int
}

type TicketRepository interface {
	Create(ctx context.Context, ticket *Ticket) error
	GetByKey(ctx context.Context, key string) (Ticket, error)
	CompletePending(ctx context.Context, id uint) (bool, error)
	SetAssignee(ctx context.Context, id uint, assignee string) (bool, error)
	SetEmailSentAt(ctx context.Context, id uint, sentAt time.Time) error
	List(ctx context.Context, filter TicketFilter) ([]Ticket, int64, error)
}
