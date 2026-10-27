package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Storage interface {
	Upload(ctx context.Context, key, contentType string, reader io.Reader, size int64) error
	Download(ctx context.Context, key string) (io.ReadCloser, string, error)
	Delete(ctx context.Context, key string) error
}

type LocalStorage struct{ root string }

func NewLocalStorage(root string) (*LocalStorage, error) {
	if err := os.MkdirAll(root, 0750); err != nil {
		return nil, fmt.Errorf("create storage directory: %w", err)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &LocalStorage{root: root}, nil
}

func (s *LocalStorage) path(key string) (string, error) {
	if key == "" || filepath.IsAbs(key) || strings.Contains(key, `\`) {
		return "", fmt.Errorf("invalid storage key")
	}
	path := filepath.Join(s.root, filepath.Clean(key))
	rel, err := filepath.Rel(s.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid storage key")
	}
	return path, nil
}

func (s *LocalStorage) Upload(ctx context.Context, key, contentType string, reader io.Reader, size int64) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("create attachment directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0640)
	if err != nil {
		return fmt.Errorf("create attachment: %w", err)
	}
	if _, err = io.Copy(f, reader); err != nil {
		f.Close()
		os.Remove(path)
		return fmt.Errorf("write attachment: %w", err)
	}
	return f.Close()
}

func (s *LocalStorage) Download(ctx context.Context, key string) (io.ReadCloser, string, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open attachment: %w", err)
	}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return f, contentType, nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete attachment: %w", err)
	}
	return nil
}

// MemoryStorage for testing
type MemoryStorage struct {
	mu    sync.RWMutex
	files map[string][]byte
	types map[string]string
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		files: make(map[string][]byte),
		types: make(map[string]string),
	}
}

func (m *MemoryStorage) Upload(ctx context.Context, key, contentType string, reader io.Reader, size int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.files[key] = data
	m.types[key] = contentType
	return nil
}

func (m *MemoryStorage) Download(ctx context.Context, key string) (io.ReadCloser, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.files[key]
	if !ok {
		return nil, "", fmt.Errorf("file not found: %s", key)
	}
	contentType := m.types[key]
	return io.NopCloser(bytes.NewReader(data)), contentType, nil
}

func (m *MemoryStorage) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, key)
	delete(m.types, key)
	return nil
}
