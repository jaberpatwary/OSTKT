package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"ticket-system/internal/domain"
)

const MaxAttachmentBytes int64 = 10 << 20

var (
	ErrInvalidAttachment  = errors.New("invalid attachment")
	ErrAttachmentTooLarge = domain.ErrAttachmentTooLarge
)

type AttachmentService struct {
	repository domain.AttachmentRepository
	storage    domain.AttachmentStorage
}

func NewAttachmentService(repository domain.AttachmentRepository, storage domain.AttachmentStorage) *AttachmentService {
	return &AttachmentService{repository: repository, storage: storage}
}

func (s *AttachmentService) Upload(ctx context.Context, actor domain.User, workType string, workID uint, filename string, source io.Reader) (domain.Attachment, error) {
	workType = strings.ToLower(strings.TrimSpace(workType))
	if (workType != "ticket" && workType != "task") || workID == 0 || source == nil {
		return domain.Attachment{}, ErrInvalidAttachment
	}
	name, extension, err := cleanAttachmentName(filename)
	if err != nil {
		return domain.Attachment{}, err
	}
	actorName := strings.TrimSpace(actor.DisplayName)
	if actorName == "" {
		actorName = strings.TrimSpace(actor.Username)
	}
	if actor.ID == 0 || actorName == "" || strings.TrimSpace(actor.Role) == "" {
		return domain.Attachment{}, ErrInvalidAttachment
	}

	key, size, prefix, err := s.storage.Save(ctx, source, MaxAttachmentBytes)
	if err != nil {
		if errors.Is(err, domain.ErrAttachmentTooLarge) {
			return domain.Attachment{}, ErrAttachmentTooLarge
		}
		return domain.Attachment{}, err
	}
	contentType, err := validateAttachmentContent(extension, prefix)
	if err != nil {
		_ = s.storage.Delete(context.Background(), key)
		return domain.Attachment{}, err
	}
	attachment := domain.Attachment{
		WorkType: workType, WorkID: workID, OriginalName: name,
		ContentType: contentType, Size: size, StoredKey: key,
		UploaderID: actor.ID, UploadedBy: actorName, UploadedRole: actor.Role,
	}
	if err := s.repository.Create(ctx, &attachment); err != nil {
		_ = s.storage.Delete(context.Background(), key)
		return domain.Attachment{}, err
	}
	return attachment, nil
}

func (s *AttachmentService) List(ctx context.Context, workType string, workID uint) ([]domain.Attachment, error) {
	workType = strings.ToLower(strings.TrimSpace(workType))
	if (workType != "ticket" && workType != "task") || workID == 0 {
		return nil, ErrInvalidAttachment
	}
	return s.repository.List(ctx, workType, workID)
}

func (s *AttachmentService) Download(ctx context.Context, workType string, workID, attachmentID uint) (domain.Attachment, io.ReadCloser, error) {
	workType = strings.ToLower(strings.TrimSpace(workType))
	if (workType != "ticket" && workType != "task") || workID == 0 || attachmentID == 0 {
		return domain.Attachment{}, nil, ErrInvalidAttachment
	}
	attachment, err := s.repository.Get(ctx, attachmentID)
	if err != nil {
		return domain.Attachment{}, nil, err
	}
	if attachment.WorkType != workType || attachment.WorkID != workID {
		return domain.Attachment{}, nil, domain.ErrNotFound
	}
	file, err := s.storage.Open(ctx, attachment.StoredKey)
	if err != nil {
		return domain.Attachment{}, nil, err
	}
	return attachment, file, nil
}

func cleanAttachmentName(filename string) (string, string, error) {
	filename = strings.ReplaceAll(filename, `\`, "/")
	name := filepath.Base(strings.TrimSpace(filename))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || !utf8.ValidString(name) || len([]rune(name)) > 180 {
		return "", "", ErrInvalidAttachment
	}
	extension := strings.ToLower(filepath.Ext(name))
	switch extension {
	case ".pdf", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".txt", ".csv":
		return name, extension, nil
	default:
		return "", "", ErrInvalidAttachment
	}
}

func validateAttachmentContent(extension string, prefix []byte) (string, error) {
	if len(prefix) == 0 {
		return "", ErrInvalidAttachment
	}
	detected, _, _ := mime.ParseMediaType(http.DetectContentType(prefix))
	var expected string
	switch extension {
	case ".pdf":
		expected = "application/pdf"
	case ".png":
		expected = "image/png"
	case ".jpg", ".jpeg":
		expected = "image/jpeg"
	case ".gif":
		expected = "image/gif"
	case ".webp":
		expected = "image/webp"
	case ".txt", ".csv":
		for _, b := range prefix {
			if b == 0 || (b < 0x20 && b != '\n' && b != '\r' && b != '\t' && b != '\f') {
				return "", ErrInvalidAttachment
			}
		}
		if detected != "text/plain" && detected != "text/csv" && detected != "application/octet-stream" {
			return "", ErrInvalidAttachment
		}
		if extension == ".csv" {
			return "text/csv; charset=utf-8", nil
		}
		return "text/plain; charset=utf-8", nil
	default:
		return "", ErrInvalidAttachment
	}
	if extension == ".webp" && len(prefix) >= 12 && bytes.Equal(prefix[:4], []byte("RIFF")) && bytes.Equal(prefix[8:12], []byte("WEBP")) {
		detected = "image/webp"
	}
	if detected != expected {
		return "", fmt.Errorf("%w: file content does not match %s", ErrInvalidAttachment, extension)
	}
	return expected, nil
}
