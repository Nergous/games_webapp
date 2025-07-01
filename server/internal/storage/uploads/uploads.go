package uploads

import (
	"errors"
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

func (u *Uploads) ReplaceImage(image []byte, oldFilename string) error {
	if len(image) == 0 {
		return ErrInvalidImage
	}

	if oldFilename == "" {
		return ErrInvalidFileName
	}

	fullPath := filepath.Join(u.folderPath, oldFilename)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return ErrFileNotExists
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	tempPath := fullPath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return err
	}

	if _, err := file.Write(image); err != nil {
		file.Close()
		_ = os.Remove(tempPath)
		return err
	}
	file.Close()

	if err := os.Rename(tempPath, fullPath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}

	return nil
}
