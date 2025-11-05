package uploads

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrInvalidImage    = errors.New("invalid image data")
	ErrFileExists      = errors.New("file already exists")
	ErrFileNotExists   = errors.New("file does not exist")
	ErrInvalidFileName = errors.New("invalid file name")
)

type IUploads interface {
	SaveImage(image []byte, filename string) error
	DeleteImage(filename string) error
	ReplaceImage(image []byte, oldFilename, newFilename string) error
}

type Uploads struct {
	folderPath string
	mu         sync.RWMutex
}

func NewUploads(folderPath string) (*Uploads, error) {
	if folderPath == "" {
		return nil, errors.New("folder path is empty")
	}

	folderPath = filepath.Clean(folderPath) + string(filepath.Separator)

	u := &Uploads{folderPath: folderPath}

	if err := u.ensureFolderExists(); err != nil {
		return nil, err
	}

	return u, nil
}

func (u *Uploads) ensureFolderExists() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if _, err := os.Stat(u.folderPath); os.IsNotExist(err) {
		if err := os.MkdirAll(u.folderPath, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (u *Uploads) SaveImage(image []byte, filename string) error {
	if len(image) == 0 {
		return ErrInvalidImage
	}

	if filename == "" {
		return ErrInvalidFileName
	}

	fullPath := filepath.Join(u.folderPath, filename)

	u.mu.Lock()
	defer u.mu.Unlock()

	if _, err := os.Stat(fullPath); err == nil {
		return ErrFileExists
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(image); err != nil {
		_ = os.Remove(fullPath)
		return err
	}

	return nil
}

func (u *Uploads) DeleteImage(filename string) error {
	if filename == "" {
		return ErrInvalidFileName
	}

	fullPath := filepath.Join(u.folderPath, filename)

	u.mu.Lock()
	defer u.mu.Unlock()

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return ErrFileNotExists
	}

	return os.Remove(fullPath)
}

func (u *Uploads) ReplaceImage(image []byte, oldFilename, newFilename string) error {
	if len(image) == 0 {
		return ErrInvalidImage
	}

	if oldFilename == "" || newFilename == "" {
		return ErrInvalidFileName
	}

	oldPath := filepath.Join(u.folderPath, oldFilename)
	newPath := filepath.Join(u.folderPath, newFilename)

	// Проверяем, существует ли старый файл (если его нужно удалить)
	if _, err := os.Stat(oldPath); oldFilename != newFilename && os.IsNotExist(err) {
		return ErrFileNotExists
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	// Создаем временный файл для безопасной записи
	tempPath := newPath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer file.Close()

	// Записываем данные во временный файл
	if _, err := file.Write(image); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to write image data: %w", err)
	}

	// Закрываем файл перед операциями переименования/удаления
	file.Close()

	// Атомарно заменяем новый файл (если он существует)
	if err := os.Rename(tempPath, newPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	// Удаляем старый файл, если он отличается от нового
	if oldFilename != newFilename {
		if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove old file: %w", err)
		}
	}

	return nil
}
