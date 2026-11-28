package usecase

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"ticket-system/internal/domain"
)

const (
	RoleAdmin   = "admin"
	RoleManager = "manager"
	RoleAgent   = "agent"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidUser        = errors.New("invalid user details")
	ErrWeakPassword       = errors.New("password must be at least 12 characters and at most 72 bytes")
	ErrLastAdmin          = domain.ErrLastAdmin
	ErrUsernameTaken      = domain.ErrDuplicateKey
	usernamePattern       = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._@+-]{2,63}$`)
)

type SecurityService struct{ repository domain.SecurityRepository }

func NewSecurityService(repository domain.SecurityRepository) *SecurityService {
	return &SecurityService{repository: repository}
}

func (s *SecurityService) BootstrapAdmin(ctx context.Context, username, password string) error {
	hasUsers, err := s.repository.HasUsers(ctx)
	if err != nil {
		return err
	}
	if hasUsers {
		return nil
	}
	username = strings.ToLower(strings.TrimSpace(username))
	password = strings.TrimSpace(password)
	if !usernamePattern.MatchString(username) || password == "" || len(password) > 72 {
		return ErrInvalidUser
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repository.BootstrapAdmin(ctx, username, username, string(hash))
}

func (s *SecurityService) Authenticate(ctx context.Context, username, password string) (domain.User, error) {
	if len(password) == 0 || len(password) > 72 {
		return domain.User{}, ErrInvalidCredentials
	}
	user, passwordHash, err := s.repository.FindUserByUsername(ctx, username)
	if err != nil || !user.Active || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return domain.User{}, ErrInvalidCredentials
	}
	return user, nil
}

func (s *SecurityService) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.repository.ListUsers(ctx)
}

func (s *SecurityService) CreateUser(ctx context.Context, input domain.UserInput) (domain.User, error) {
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	if !usernamePattern.MatchString(input.Username) || input.DisplayName == "" || len([]rune(input.DisplayName)) > 120 || !validRole(input.Role) {
		return domain.User{}, ErrInvalidUser
	}
	if utf8.RuneCountInString(input.Password) < 12 || len(input.Password) > 72 {
		return domain.User{}, ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}
	input.Active = true
	return s.repository.CreateUser(ctx, input, string(hash))
}

func (s *SecurityService) UpdateUser(ctx context.Context, id uint, input domain.UserUpdate) (domain.User, error) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	if input.DisplayName == "" || len([]rune(input.DisplayName)) > 120 || !validRole(input.Role) {
		return domain.User{}, ErrInvalidUser
	}
	var passwordHash string
	if input.Password != "" {
		if utf8.RuneCountInString(input.Password) < 12 || len(input.Password) > 72 {
			return domain.User{}, ErrWeakPassword
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return domain.User{}, err
		}
		passwordHash = string(hash)
	}
	return s.repository.UpdateUser(ctx, id, input, passwordHash)
}

func (s *SecurityService) RecordAudit(ctx context.Context, event domain.AuditEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	return s.repository.CreateAudit(ctx, event)
}

func (s *SecurityService) ListAudit(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	return s.repository.ListAudit(ctx, limit)
}

func validRole(role string) bool {
	switch role {
	case RoleAdmin, RoleManager, RoleAgent:
		return true
	default:
		return false
	}
}
