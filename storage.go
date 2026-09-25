package imagetpl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// BackgroundStore keeps the original uploaded image between process restarts.
// Load should return os.ErrNotExist when no image has been saved yet.
type BackgroundStore interface {
	Load(context.Context) ([]byte, error)
	Save(context.Context, []byte) error
}

// UseBackgroundStore loads a saved background and uses the store for uploads.
// A failed load leaves the page unchanged and returns the error to the caller.
func (p *ImageTemplatePage) UseBackgroundStore(store BackgroundStore) error {
	if store == nil {
		return errors.New("background store is nil")
	}
	data, err := store.Load(context.Background())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load background: %w", err)
	}
	if err == nil {
		width, height, validationErr := validateBackground(data)
		if validationErr != nil {
			return fmt.Errorf("stored background is invalid: %w", validationErr)
		}
		p.setBackground(data, width, height, true)
	}
	p.mu.Lock()
	p.backgroundStore = store
	p.mu.Unlock()
	return nil
}

// FileBackgroundStore saves one background at a caller-selected path.
type FileBackgroundStore struct {
	Path string
}

func (s FileBackgroundStore) Load(_ context.Context) ([]byte, error) {
	return os.ReadFile(s.Path)
}

func (s FileBackgroundStore) Save(_ context.Context, data []byte) error {
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".background-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), s.Path)
}
