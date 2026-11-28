package domain

import "context"

type SecurityRepository interface {
	Migrate() error
	HasUsers(ctx context.Context) (bool, error)
	BootstrapAdmin(ctx context.Context, username, displayName, passwordHash string) error
	FindUserByUsername(ctx context.Context, username string) (User, string, error)
	GetUser(ctx context.Context, id uint) (User, error)
	ListUsers(ctx context.Context) ([]User, error)
	CreateUser(ctx context.Context, input UserInput, passwordHash string) (User, error)
	UpdateUser(ctx context.Context, id uint, input UserUpdate, passwordHash string) (User, error)
	CreateAudit(ctx context.Context, event AuditEvent) error
	ListAudit(ctx context.Context, limit int) ([]AuditEvent, error)
}
