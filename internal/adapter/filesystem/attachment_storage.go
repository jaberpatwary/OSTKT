package filesystem

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"ticket-system/internal/domain"
)

var storedAttachmentKey = regexp.MustCompile(`^[a-f0-9]{32}$`)

type AttachmentStorage struct{ dir string }

func NewAttachmentStorage(dir string) (*AttachmentStorage, error) {
	if dir == "" {
		return nil, errors.New("attachment directory is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create attachment directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure attachment directory: %w", err)
	}
	absoluteDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve attachment directory: %w", err)
	}
	return &AttachmentStorage{dir: absoluteDir}, nil
}

func (s *AttachmentStorage) Save(ctx context.Context, source io.Reader, maxBytes int64) (string, int64, []byte, error) {
	if source == nil || maxBytes <= 0 {
		return "", 0, nil, errors.New("invalid attachment input")
	}
	if err := ctx.Err(); err != nil {
		return "", 0, nil, err
	}
	file, err := os.CreateTemp(s.dir, ".upload-*")
	if err != nil {
		return "", 0, nil, fmt.Errorf("create temporary attachment: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	size, copyErr := io.Copy(file, io.LimitReader(&contextReader{ctx: ctx, reader: source}, maxBytes+1))
	if copyErr != nil {
		_ = file.Close()
		return "", 0, nil, copyErr
	}
	if size > maxBytes {
		_ = file.Close()
		return "", size, nil, fmt.Errorf("%w: limit is %d bytes", domain.ErrAttachmentTooLarge, maxBytes)
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return "", 0, nil, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return "", 0, nil, fmt.Errorf("flush attachment: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return "", 0, nil, fmt.Errorf("secure attachment: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return "", 0, nil, fmt.Errorf("read attachment header: %w", err)
	}
	prefix := make([]byte, 512)
	n, readErr := file.Read(prefix)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		_ = file.Close()
		return "", 0, nil, fmt.Errorf("read attachment header: %w", readErr)
	}
	prefix = prefix[:n]
	if err := file.Close(); err != nil {
		return "", 0, nil, fmt.Errorf("close attachment: %w", err)
	}
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", 0, nil, fmt.Errorf("generate attachment key: %w", err)
	}
	key := hex.EncodeToString(keyBytes)
	if err := os.Rename(tempPath, filepath.Join(s.dir, key)); err != nil {
		return "", 0, nil, fmt.Errorf("store attachment: %w", err)
	}
	return key, size, prefix, nil
}

func (s *AttachmentStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !storedAttachmentKey.MatchString(key) {
		return nil, errors.New("invalid stored attachment key")
	}
	return os.Open(filepath.Join(s.dir, key))
}

func (s *AttachmentStorage) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !storedAttachmentKey.MatchString(key) {
		return errors.New("invalid stored attachment key")
	}
	err := os.Remove(filepath.Join(s.dir, key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
