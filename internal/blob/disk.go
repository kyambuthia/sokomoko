package blob

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DiskStore stores binary assets as files on a filesystem mount.
type DiskStore struct {
	baseDir string
}

// NewDiskStore returns a DiskStore anchored at baseDir.
func NewDiskStore(baseDir string) (*DiskStore, error) {
	cleanDir := filepath.Clean(baseDir)
	if err := os.MkdirAll(cleanDir, 0750); err != nil {
		return nil, fmt.Errorf("blob: failed to create base dir: %w", err)
	}
	return &DiskStore{baseDir: cleanDir}, nil
}

func (d *DiskStore) filePath(key string) (string, error) {
	clean, err := CleanKey(key)
	if err != nil {
		return "", err
	}
	targetPath := filepath.Join(d.baseDir, filepath.FromSlash(clean))
	rel, err := filepath.Rel(d.baseDir, targetPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", ErrInvalidKey
	}
	return targetPath, nil
}

func (d *DiskStore) metaPath(targetPath string) string {
	return targetPath + ".meta"
}

func (d *DiskStore) Get(_ context.Context, key string) ([]byte, string, error) {
	path, err := d.filePath(key)
	if err != nil {
		return nil, "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}

	metaBytes, err := os.ReadFile(d.metaPath(path))
	contentType := "application/octet-stream"
	if err == nil && len(metaBytes) > 0 {
		contentType = strings.TrimSpace(string(metaBytes))
	}
	return data, contentType, nil
}

func (d *DiskStore) Put(_ context.Context, key string, contentType string, data []byte) error {
	path, err := d.filePath(key)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return ErrInvalidPayload
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, "upload-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpName, path); err != nil {
		return err
	}

	if contentType != "" {
		_ = os.WriteFile(d.metaPath(path), []byte(strings.TrimSpace(contentType)), 0640)
	}
	return nil
}

func (d *DiskStore) Delete(_ context.Context, key string) error {
	path, err := d.filePath(key)
	if err != nil {
		return err
	}

	_ = os.Remove(d.metaPath(path))
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (d *DiskStore) Exists(_ context.Context, key string) (bool, error) {
	path, err := d.filePath(key)
	if err != nil {
		return false, err
	}

	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return !info.IsDir(), nil
}
