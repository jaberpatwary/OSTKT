package domain

import (
	"context"
	"time"
)

type LoginSession struct {
	UserID    uint
	TokenHash string
	CSRFHash  string
	ExpiresAt time.Time
}

type SessionCredentials struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
	User      User
}

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
	CreateSession(ctx context.Context, session LoginSession) error
	GetSession(ctx context.Context, tokenHash string) (LoginSession, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteSessionsForUser(ctx context.Context, userID uint) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) error
	GetMFA(ctx context.Context, userID uint) (secretCiphertext string, enabled bool, err error)
	SetMFA(ctx context.Context, userID uint, secretCiphertext string, enabled bool) error
}
