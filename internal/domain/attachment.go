package domain

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrAttachmentTooLarge = errors.New("attachment exceeds size limit")

type Attachment struct {
	ID           uint      `json:"id"`
	WorkType     string    `json:"work_type"`
	WorkID       uint      `json:"work_id"`
	OriginalName string    `json:"original_name"`
	ContentType  string    `json:"content_type"`
	Size         int64     `json:"size"`
	UploadedBy   string    `json:"uploaded_by"`
	UploadedRole string    `json:"uploaded_role"`
	StoredKey    string    `json:"-"`
	UploaderID   uint      `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type AttachmentRepository interface {
	Migrate() error
	Create(ctx context.Context, attachment *Attachment) error
	List(ctx context.Context, workType string, workID uint) ([]Attachment, error)
	Get(ctx context.Context, attachmentID uint) (Attachment, error)
}

type AttachmentStorage interface {
	Save(ctx context.Context, source io.Reader, maxBytes int64) (storedKey string, size int64, prefix []byte, err error)
	Open(ctx context.Context, storedKey string) (io.ReadCloser, error)
	Delete(ctx context.Context, storedKey string) error
}
