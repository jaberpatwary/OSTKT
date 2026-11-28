package postgres

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"ticket-system/internal/domain"
)

type taskRecord struct {
	ID          uint   `gorm:"primaryKey"`
	Title       string `gorm:"size:160;not null;index"`
	Description string `gorm:"type:text"`
	CreatedBy   string `gorm:"size:256;not null;default:'';index"`
	AssignedTo  string `gorm:"size:100;index"`
	Priority    string `gorm:"size:16;not null;default:medium;index"`
	DueDate     string `gorm:"size:10;index"`
	Status      string `gorm:"size:20;not null;default:pending;index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (taskRecord) TableName() string { return "tasks" }

func (r taskRecord) toDomain() domain.Task {
	return domain.Task{
		ID: r.ID, Title: r.Title, Description: r.Description, CreatedBy: r.CreatedBy,
		AssignedTo: r.AssignedTo, Priority: r.Priority, DueDate: r.DueDate,
		Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

type TaskRepository struct {
	db *gorm.DB
}

func NewTaskRepository(db *gorm.DB) *TaskRepository { return &TaskRepository{db: db} }

func (r *TaskRepository) Migrate() error {
	return r.db.AutoMigrate(&taskRecord{})
}

func (r *TaskRepository) Create(ctx context.Context, task *domain.Task) error {
	record := taskRecord{
		Title: task.Title, Description: task.Description, CreatedBy: task.CreatedBy,
		AssignedTo: task.AssignedTo, Priority: task.Priority,
		DueDate: task.DueDate, Status: task.Status,
	}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return err
	}
	*task = record.toDomain()
	return nil
}

func (r *TaskRepository) Get(ctx context.Context, id uint) (domain.Task, error) {
	var record taskRecord
	if err := r.db.WithContext(ctx).First(&record, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return domain.Task{}, domain.ErrNotFound
		}
		return domain.Task{}, err
	}
	return record.toDomain(), nil
}

func (r *TaskRepository) List(ctx context.Context, filter domain.TaskFilter) ([]domain.Task, int64, error) {
	query := r.db.WithContext(ctx).Model(&taskRecord{})
	if !filter.CreatedFrom.IsZero() {
		query = query.Where("created_at >= ?", filter.CreatedFrom)
	}
	if !filter.CreatedUntil.IsZero() {
		query = query.Where("created_at < ?", filter.CreatedUntil)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Assignee != "" {
		query = query.Where("assigned_to = ?", filter.Assignee)
	}
	if filter.Query != "" {
		like := "%" + strings.ToLower(filter.Query) + "%"
		query = query.Where("(LOWER(title) LIKE ? OR LOWER(description) LIKE ? OR LOWER(created_by) LIKE ? OR LOWER(assigned_to) LIKE ?)", like, like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit <= 0 || limit > 10000 {
		limit = 100
	}
	records := make([]taskRecord, 0)
	if err := query.Order("CASE WHEN status = 'done' THEN 1 ELSE 0 END, due_date = '', due_date ASC, created_at DESC, id DESC").Limit(limit).Offset(filter.Offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	tasks := make([]domain.Task, 0, len(records))
	for _, record := range records {
		tasks = append(tasks, record.toDomain())
	}
	return tasks, total, nil
}

func (r *TaskRepository) Update(ctx context.Context, id uint, changes map[string]any) (bool, error) {
	changes["updated_at"] = time.Now()
	result := r.db.WithContext(ctx).Model(&taskRecord{}).Where("id = ?", id).Updates(changes)
	return result.RowsAffected > 0, result.Error
}
