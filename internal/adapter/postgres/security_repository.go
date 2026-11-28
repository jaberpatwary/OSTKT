package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ticket-system/internal/domain"
)

type userRecord struct {
	ID           uint   `gorm:"primaryKey"`
	Username     string `gorm:"size:64;not null;uniqueIndex"`
	DisplayName  string `gorm:"size:120;not null"`
	Role         string `gorm:"size:20;not null;index"`
	PasswordHash string `gorm:"size:100;not null"`
	Active       bool   `gorm:"not null;default:true;index"`
	MFASecret    string `gorm:"size:512;not null;default:''"`
	MFAEnabled   bool   `gorm:"not null;default:false"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (userRecord) TableName() string { return "users" }

func (r userRecord) toDomain() domain.User {
	return domain.User{
		ID: r.ID, Username: r.Username, DisplayName: r.DisplayName, Role: r.Role,
		Active: r.Active, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		MFAEnabled: r.MFAEnabled,
	}
}

type auditRecord struct {
	ID         uint      `gorm:"primaryKey"`
	UserID     uint      `gorm:"index"`
	Username   string    `gorm:"size:64;not null;index"`
	Role       string    `gorm:"size:20;not null;index"`
	Action     string    `gorm:"size:12;not null;index"`
	Resource   string    `gorm:"size:240;not null"`
	StatusCode int       `gorm:"not null"`
	IPAddress  string    `gorm:"size:64"`
	UserAgent  string    `gorm:"size:256"`
	CreatedAt  time.Time `gorm:"index"`
}

func (auditRecord) TableName() string { return "audit_logs" }

type sessionRecord struct {
	TokenHash string    `gorm:"primaryKey;size:64"`
	UserID    uint      `gorm:"not null;index"`
	CSRFHash  string    `gorm:"size:64;not null"`
	ExpiresAt time.Time `gorm:"not null;index"`
	CreatedAt time.Time
}

func (sessionRecord) TableName() string { return "login_sessions" }

func (r sessionRecord) toDomain() domain.LoginSession {
	return domain.LoginSession{TokenHash: r.TokenHash, UserID: r.UserID, CSRFHash: r.CSRFHash, ExpiresAt: r.ExpiresAt}
}

func (r auditRecord) toDomain() domain.AuditEvent {
	return domain.AuditEvent{
		ID: r.ID, UserID: r.UserID, Username: r.Username, Role: r.Role,
		Action: r.Action, Resource: r.Resource, StatusCode: r.StatusCode,
		IPAddress: r.IPAddress, UserAgent: r.UserAgent, CreatedAt: r.CreatedAt,
	}
}

type SecurityRepository struct{ db *gorm.DB }

func NewSecurityRepository(db *gorm.DB) *SecurityRepository {
	return &SecurityRepository{db: db}
}

func (r *SecurityRepository) Migrate() error {
	return r.db.AutoMigrate(&userRecord{}, &auditRecord{}, &sessionRecord{})
}

func (r *SecurityRepository) HasUsers(ctx context.Context) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&userRecord{}).Count(&count).Error
	return count > 0, err
}

func (r *SecurityRepository) BootstrapAdmin(ctx context.Context, username, displayName, passwordHash string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&userRecord{}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&userRecord{
			Username: username, DisplayName: displayName, Role: "admin",
			PasswordHash: passwordHash, Active: true,
		}).Error
	})
}

func (r *SecurityRepository) FindUserByUsername(ctx context.Context, username string) (domain.User, string, error) {
	var record userRecord
	err := r.db.WithContext(ctx).Where("username = ?", strings.ToLower(strings.TrimSpace(username))).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.User{}, "", domain.ErrNotFound
	}
	return record.toDomain(), record.PasswordHash, err
}

func (r *SecurityRepository) GetUser(ctx context.Context, id uint) (domain.User, error) {
	var record userRecord
	if err := r.db.WithContext(ctx).First(&record, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.User{}, domain.ErrNotFound
		}
		return domain.User{}, err
	}
	return record.toDomain(), nil
}

func (r *SecurityRepository) ListUsers(ctx context.Context) ([]domain.User, error) {
	records := make([]userRecord, 0)
	if err := r.db.WithContext(ctx).Order("role, username").Find(&records).Error; err != nil {
		return nil, err
	}
	users := make([]domain.User, 0, len(records))
	for _, record := range records {
		users = append(users, record.toDomain())
	}
	return users, nil
}

func (r *SecurityRepository) CreateUser(ctx context.Context, input domain.UserInput, passwordHash string) (domain.User, error) {
	record := userRecord{
		Username:    strings.ToLower(strings.TrimSpace(input.Username)),
		DisplayName: strings.TrimSpace(input.DisplayName), Role: input.Role,
		PasswordHash: passwordHash, Active: input.Active,
	}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || isDuplicate(err) {
			return domain.User{}, domain.ErrDuplicateKey
		}
		return domain.User{}, err
	}
	return record.toDomain(), nil
}

func (r *SecurityRepository) UpdateUser(ctx context.Context, id uint, input domain.UserUpdate, passwordHash string) (domain.User, error) {
	var updated userRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current userRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		if current.Active && current.Role == "admin" && (!input.Active || input.Role != "admin") {
			var admins int64
			if err := tx.Model(&userRecord{}).Where("active = ? AND role = ?", true, "admin").Count(&admins).Error; err != nil {
				return err
			}
			if admins <= 1 {
				return domain.ErrLastAdmin
			}
		}
		changes := map[string]any{
			"display_name": strings.TrimSpace(input.DisplayName),
			"role":         input.Role,
			"active":       input.Active,
		}
		if passwordHash != "" {
			changes["password_hash"] = passwordHash
		}
		if err := tx.Model(&current).Updates(changes).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&sessionRecord{}).Error; err != nil {
			return err
		}
		return tx.First(&updated, id).Error
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return domain.User{}, err
	}
	return updated.toDomain(), nil
}

func (r *SecurityRepository) CreateAudit(ctx context.Context, event domain.AuditEvent) error {
	record := auditRecord{
		UserID: event.UserID, Username: event.Username, Role: event.Role,
		Action: event.Action, Resource: event.Resource, StatusCode: event.StatusCode,
		IPAddress: event.IPAddress, UserAgent: event.UserAgent, CreatedAt: event.CreatedAt,
	}
	if event.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Create(&record).Error
}

func (r *SecurityRepository) ListAudit(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	records := make([]auditRecord, 0)
	if err := r.db.WithContext(ctx).Order("created_at DESC, id DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	events := make([]domain.AuditEvent, 0, len(records))
	for _, record := range records {
		events = append(events, record.toDomain())
	}
	return events, nil
}

func (r *SecurityRepository) CreateSession(ctx context.Context, session domain.LoginSession) error {
	return r.db.WithContext(ctx).Create(&sessionRecord{
		TokenHash: session.TokenHash, UserID: session.UserID, CSRFHash: session.CSRFHash, ExpiresAt: session.ExpiresAt,
	}).Error
}

func (r *SecurityRepository) GetSession(ctx context.Context, tokenHash string) (domain.LoginSession, error) {
	var record sessionRecord
	if err := r.db.WithContext(ctx).First(&record, "token_hash = ?", tokenHash).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.LoginSession{}, domain.ErrNotFound
		}
		return domain.LoginSession{}, err
	}
	return record.toDomain(), nil
}

func (r *SecurityRepository) DeleteSession(ctx context.Context, tokenHash string) error {
	return r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).Delete(&sessionRecord{}).Error
}

func (r *SecurityRepository) DeleteSessionsForUser(ctx context.Context, userID uint) error {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&sessionRecord{}).Error
}

func (r *SecurityRepository) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	return r.db.WithContext(ctx).Where("expires_at <= ?", now).Delete(&sessionRecord{}).Error
}

func (r *SecurityRepository) GetMFA(ctx context.Context, userID uint) (string, bool, error) {
	var record userRecord
	if err := r.db.WithContext(ctx).First(&record, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, domain.ErrNotFound
		}
		return "", false, err
	}
	return record.MFASecret, record.MFAEnabled, nil
}

func (r *SecurityRepository) SetMFA(ctx context.Context, userID uint, secretCiphertext string, enabled bool) error {
	result := r.db.WithContext(ctx).Model(&userRecord{}).Where("id = ?", userID).Updates(map[string]any{
		"mfa_secret": secretCiphertext, "mfa_enabled": enabled,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
