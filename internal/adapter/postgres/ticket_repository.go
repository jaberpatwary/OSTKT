package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ticket-system/internal/domain"
)

type ticketRecord struct {
	ID           uint   `gorm:"primaryKey"`
	TicketNumber string `gorm:"uniqueIndex;not null"`
	EmployeeName string `gorm:"not null"`
	EmployeeID   string
	Department   string
	Problem      string
	AssignedTo   string
	Email        string
	Phone        string
	Status       string `gorm:"default:pending"`
	EmailSentAt  *time.Time
	CreatedAt    time.Time
}

func (ticketRecord) TableName() string { return "tickets" }

type ticketNumberCounter struct {
	ID        uint `gorm:"primaryKey"`
	LastValue uint64
}

func (ticketNumberCounter) TableName() string { return "ticket_number_counters" }

func recordFromDomain(ticket *domain.Ticket) ticketRecord {
	return ticketRecord{
		ID: ticket.ID, TicketNumber: ticket.TicketNumber,
		EmployeeName: ticket.EmployeeName, EmployeeID: ticket.EmployeeID,
		Department: ticket.Department, Problem: ticket.Problem,
		AssignedTo: ticket.AssignedTo, Email: ticket.Email, Phone: ticket.Phone,
		Status:      ticket.Status,
		EmailSentAt: ticket.EmailSentAt, CreatedAt: ticket.CreatedAt,
	}
}

func (record ticketRecord) toDomain() domain.Ticket {
	return domain.Ticket{
		ID: record.ID, TicketNumber: record.TicketNumber,
		EmployeeName: record.EmployeeName, EmployeeID: record.EmployeeID,
		Department: record.Department, Problem: record.Problem,
		AssignedTo: record.AssignedTo, Email: record.Email, Phone: record.Phone,
		Status:      record.Status,
		EmailSentAt: record.EmailSentAt, CreatedAt: record.CreatedAt,
	}
}

type TicketRepository struct {
	db *gorm.DB
}

func NewTicketRepository(db *gorm.DB) *TicketRepository {
	return &TicketRepository{db: db}
}

func (r *TicketRepository) Migrate() error {
	if err := r.db.AutoMigrate(&ticketRecord{}, &ticketNumberCounter{}); err != nil {
		return err
	}
	var count int64
	if err := r.db.Model(&ticketRecord{}).Count(&count).Error; err != nil {
		return err
	}
	counter := ticketNumberCounter{ID: 1, LastValue: uint64(count)}
	return r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&counter).Error
}

func (r *TicketRepository) Create(ctx context.Context, ticket *domain.Ticket) error {
	record := recordFromDomain(ticket)
	var created ticketRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var counter ticketNumberCounter
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&counter, 1).Error; err != nil {
			return err
		}
		counter.LastValue++
		if err := tx.Model(&counter).Update("last_value", counter.LastValue).Error; err != nil {
			return err
		}
		record.TicketNumber = fmt.Sprintf("TKT-%06d", counter.LastValue)
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		created = record
		return nil
	})
	if errors.Is(err, gorm.ErrDuplicatedKey) || isDuplicate(err) {
		return domain.ErrDuplicateKey
	}
	if err == nil {
		*ticket = created.toDomain()
	}
	return err
}

func (r *TicketRepository) GetByKey(ctx context.Context, key string) (domain.Ticket, error) {
	var record ticketRecord
	db := r.db.WithContext(ctx)
	key = strings.TrimSpace(key)
	var err error
	if id, parseErr := strconv.ParseUint(key, 10, 64); parseErr == nil {
		err = db.First(&record, id).Error
	} else {
		err = db.Where("ticket_number = ?", strings.ToUpper(key)).First(&record).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Ticket{}, domain.ErrNotFound
	}
	return record.toDomain(), err
}

func (r *TicketRepository) CompletePending(ctx context.Context, id uint) (bool, error) {
	result := r.db.WithContext(ctx).Model(&ticketRecord{}).
		Where("id = ? AND status = ?", id, domain.StatusPending).
		Update("status", domain.StatusDone)
	return result.RowsAffected > 0, result.Error
}

func (r *TicketRepository) SetAssignee(ctx context.Context, id uint, assignee string) (bool, error) {
	result := r.db.WithContext(ctx).Model(&ticketRecord{}).
		Where("id = ?", id).
		Update("assigned_to", assignee)
	return result.RowsAffected > 0, result.Error
}

func (r *TicketRepository) SetEmailSentAt(ctx context.Context, id uint, sentAt time.Time) error {
	return r.db.WithContext(ctx).Model(&ticketRecord{}).Where("id = ?", id).Update("email_sent_at", sentAt).Error
}

func (r *TicketRepository) List(ctx context.Context, filter domain.TicketFilter) ([]domain.Ticket, int64, error) {
	query := r.db.WithContext(ctx).Model(&ticketRecord{})
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	records := make([]ticketRecord, 0)
	if err := query.Order("created_at DESC, id DESC").Limit(filter.Limit).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	tickets := make([]domain.Ticket, 0, len(records))
	for _, record := range records {
		tickets = append(tickets, record.toDomain())
	}
	return tickets, total, nil
}

func isDuplicate(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}
