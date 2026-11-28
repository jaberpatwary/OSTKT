package postgres

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"ticket-system/internal/domain"
)

type attachmentRecord struct {
	ID           uint      `gorm:"primaryKey"`
	WorkType     string    `gorm:"size:12;not null;index:idx_attachments_work"`
	WorkID       uint      `gorm:"not null;index:idx_attachments_work"`
	OriginalName string    `gorm:"size:180;not null"`
	ContentType  string    `gorm:"size:100;not null"`
	Size         int64     `gorm:"not null"`
	StoredKey    string    `gorm:"size:64;not null;uniqueIndex"`
	UploaderID   uint      `gorm:"index"`
	UploadedBy   string    `gorm:"size:120;not null"`
	UploadedRole string    `gorm:"size:20;not null"`
	CreatedAt    time.Time `gorm:"not null;autoCreateTime"`
}

func (attachmentRecord) TableName() string { return "attachments" }

func (r attachmentRecord) toDomain() domain.Attachment {
	return domain.Attachment{
		ID: r.ID, WorkType: r.WorkType, WorkID: r.WorkID,
		OriginalName: r.OriginalName, ContentType: r.ContentType,
		Size: r.Size, StoredKey: r.StoredKey, UploaderID: r.UploaderID,
		UploadedBy: r.UploadedBy, UploadedRole: r.UploadedRole,
		CreatedAt: r.CreatedAt,
	}
}

type AttachmentRepository struct{ db *gorm.DB }

func NewAttachmentRepository(db *gorm.DB) *AttachmentRepository {
	return &AttachmentRepository{db: db}
}

func (r *AttachmentRepository) Migrate() error {
	return r.db.AutoMigrate(&attachmentRecord{})
}

func (r *AttachmentRepository) Create(ctx context.Context, attachment *domain.Attachment) error {
	record := attachmentRecord{
		WorkType: attachment.WorkType, WorkID: attachment.WorkID,
		OriginalName: attachment.OriginalName, ContentType: attachment.ContentType,
		Size: attachment.Size, StoredKey: attachment.StoredKey,
		UploaderID: attachment.UploaderID, UploadedBy: attachment.UploadedBy,
		UploadedRole: attachment.UploadedRole,
	}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return err
	}
	*attachment = record.toDomain()
	return nil
}

func (r *AttachmentRepository) List(ctx context.Context, workType string, workID uint) ([]domain.Attachment, error) {
	records := make([]attachmentRecord, 0)
	if err := r.db.WithContext(ctx).Where("work_type = ? AND work_id = ?", workType, workID).
		Order("created_at ASC, id ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	attachments := make([]domain.Attachment, 0, len(records))
	for _, record := range records {
		attachments = append(attachments, record.toDomain())
	}
	return attachments, nil
}

func (r *AttachmentRepository) Get(ctx context.Context, attachmentID uint) (domain.Attachment, error) {
	var record attachmentRecord
	if err := r.db.WithContext(ctx).First(&record, attachmentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Attachment{}, domain.ErrNotFound
		}
		return domain.Attachment{}, err
	}
	return record.toDomain(), nil
}
