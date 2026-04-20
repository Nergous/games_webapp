// internal/storage/uploads/uploads.go
package uploads

import (
	"crypto/sha256"
	"fmt"
	g_errors "games_webapp/internal/errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// TYPES
// ============================================================================

// Uploads manages file operations for user-uploaded images.
//
// It provides thread-safe operations for saving, deleting, replacing, and
// downloading images to/from a designated folder on the filesystem. The
// structure handles image validation, filename generation, and ensures
// data integrity through mutex-based synchronization.
type Uploads struct {
	// folderPath is the absolute or relative path to the directory where
	// uploaded images are stored. It always ends with a path separator.
	folderPath string

	// mu protects concurrent access to filesystem operations, preventing
	// race conditions during read/write operations on the same file.
	mu sync.RWMutex
}

// ============================================================================
// INITIALIZATION
// ============================================================================

// NewUploads creates and initializes a new Uploads instance.
//
// It validates the provided folder path, normalizes it by cleaning and
// appending a trailing separator, and ensures the target directory exists
// on the filesystem. If the directory doesn't exist, it attempts to create it.
//
// Input parameters:
//   - folderPath: path to the directory where uploaded images will be stored; can be absolute or relative path, empty path is not allowed
//
// Output parameters:
//   - *Uploads: initialized Uploads instance with synchronized access control
//   - error: nil on success, or an error if folderPath is empty or directory creation fails (permission issues, invalid path)
func NewUploads(folderPath string) (*Uploads, error) {
	const op = "storage.NewUploads"
	if folderPath == "" {
		return nil, g_errors.NewWithInfo(
			op,
			g_errors.CodeInternal,
			"",
			map[string]any{
				"info": g_errors.EmptyFolderPath,
			},
		)
	}

	folderPath = filepath.Clean(folderPath) + string(filepath.Separator)

	u := &Uploads{folderPath: folderPath, mu: sync.RWMutex{}}

	if err := u.ensureFolderExists(); err != nil {
		return nil, g_errors.Wrap(
			op,
			g_errors.CodeInternal,
			"",
			err,
		)
	}

	return u, nil
}

// ============================================================================
// SETTERS
// ============================================================================

// SetFolderPath updates the uploads directory path.
//
// Note: This method does not validate the new path or create the directory.
// The caller should ensure the new path is valid and accessible before
// performing file operations.
//
// Input parameters:
//   - folderPath: new path for the uploads directory
func (u *Uploads) SetFolderPath(folderPath string) { u.folderPath = folderPath }

// ============================================================================
// METHODS
// ============================================================================

// SaveImage writes an image to the filesystem with the specified filename.
//
// It performs validation on input parameters, checks for existing files to
// prevent overwrites, and ensures thread-safe file creation. The operation
// is atomic: if writing fails, any partially created file is removed.
//
// Input parameters:
//   - image: byte slice containing the image data; must not be empty
//   - filename: name of the file to create (without path); must not be empty
//
// Output parameters:
//   - error: nil on success, or an error if image data is empty, filename is empty, file already exists, file creation fails, or writing to file fails
func (u *Uploads) SaveImage(image []byte, filename string) error {
	const op = "storage.Uploads.SaveImage"

	if len(image) == 0 {
		return g_errors.New(op, g_errors.CodeInvalidInput, g_errors.NullImageLength)
	}

	if filename == "" {
		return g_errors.New(op, g_errors.CodeInvalidInput, g_errors.EmptyFileName)
	}

	fullPath := filepath.Join(u.folderPath, filename)

	u.mu.Lock()
	defer u.mu.Unlock()

	if _, err := os.Stat(fullPath); err == nil {
		return g_errors.NewWithInfo(
			op,
			g_errors.CodeInternal,
			g_errors.FileAlreadyExists,
			map[string]any{
				"fullPath": fullPath,
			},
		)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return g_errors.Wrap(
			op,
			g_errors.CodeInternal,
			g_errors.CannotCreateFile,
			err,
		)
	}
	defer file.Close()
	if _, err := file.Write(image); err != nil {
		_ = os.Remove(fullPath)
		return g_errors.Wrap(
			op,
			g_errors.CodeInternal,
			g_errors.CannotWriteFile,
			err,
		)
	}

	return nil
}

// DeleteImage removes an image file from the filesystem.
//
// It validates the filename, checks if the file exists, and performs
// thread-safe deletion. The operation distinguishes between "file not found"
// and other filesystem errors for proper error handling.
//
// Input parameters:
//   - filename: name of the file to delete (without path); must not be empty
//
// Output parameters:
//   - error: nil on success, or an error if filename is empty, file doesn't exist (CodeNotFound), filesystem access fails, or deletion fails (permission issues, etc.)
func (u *Uploads) DeleteImage(filename string) error {
	const op = "storage.Uploads.DeleteImage"

	if filename == "" {
		return g_errors.New(op, g_errors.CodeInternal, g_errors.EmptyFileName)
	}

	fullPath := filepath.Join(u.folderPath, filename)

	u.mu.Lock()
	defer u.mu.Unlock()

	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return g_errors.WrapWithInfo(
				op,
				g_errors.CodeNotFound,
				g_errors.FileNotFound,
				map[string]any{
					"path": fullPath,
				},
				err,
			)
		} else {
			return g_errors.WrapWithInfo(
				op,
				g_errors.CodeInternal,
				g_errors.CannotDeleteFile,
				map[string]any{
					"path": fullPath,
				},
				err,
			)
		}
	}

	return os.Remove(fullPath)
}

// ReplaceImage atomically replaces an existing image with new content and/or name.
//
// This method handles three scenarios: replace content but keep the same filename
// (oldFilename == newFilename), replace content and rename file (oldFilename != newFilename),
// or create new file (if old file doesn't exist and names differ).
//
// The operation uses a temporary file to ensure atomicity: the new file is
// written completely before replacing the old one. If any step fails, the
// temporary file is cleaned up and the original file remains untouched.
//
// Input parameters:
//   - image: byte slice containing the new image data; must not be empty
//   - oldFilename: name of the existing file to replace; must not be empty
//   - newFilename: name for the new file; must not be empty
//
// Output parameters:
//   - error: nil on success, or an error if image data is empty, either filename is empty, old file doesn't exist (when replacing with different name), temporary file creation fails, writing to temporary file fails, renaming temporary file fails, or deletion of old file fails (when names differ)
func (u *Uploads) ReplaceImage(image []byte, oldFilename, newFilename string) error {
	const op = "storage.Uploads.ReplaceImage"

	if len(image) == 0 {
		return g_errors.New(op, g_errors.CodeInternal, g_errors.NullImageLength)
	}

	if oldFilename == "" || newFilename == "" {
		return g_errors.NewWithInfo(
			op,
			g_errors.CodeInternal,
			g_errors.EmptyFileName,
			map[string]any{
				"oldfilename": oldFilename,
				"newfilename": newFilename,
			},
		)
	}

	oldPath := filepath.Join(u.folderPath, oldFilename)
	newPath := filepath.Join(u.folderPath, newFilename)

	u.mu.Lock()
	defer u.mu.Unlock()

	if _, err := os.Stat(oldPath); oldFilename != newFilename && os.IsNotExist(err) {
		return g_errors.WrapWithInfo(
			op,
			g_errors.CodeNotFound,
			g_errors.FileNotFound,
			map[string]any{
				"oldfilename": oldFilename,
				"newfilename": newFilename,
			},
			err,
		)
	} else if err != nil {
		return g_errors.WrapWithInfo(
			op,
			g_errors.CodeInternal,
			g_errors.FileNotFound,
			map[string]any{
				"oldfilename": oldFilename,
				"newfilename": newFilename,
			},
			err,
		)
	}

	tempPath := newPath + ".tmp"
	file, err := os.Create(tempPath)

	if err != nil {
		return g_errors.WrapWithInfo(
			op,
			g_errors.CodeInternal,
			g_errors.CannotCreateFile,
			map[string]any{
				"tempfilename": tempPath,
			},
			err,
		)
	}
	defer file.Close()

	if _, err := file.Write(image); err != nil {
		os.Remove(tempPath)

		return g_errors.Wrap(
			op,
			g_errors.CodeInternal,
			g_errors.CannotWriteFile,
			err,
		)
	}

	file.Close()

	if err := os.Rename(tempPath, newPath); err != nil {
		os.Remove(tempPath)
		return g_errors.Wrap(
			op,
			g_errors.CodeInternal,
			g_errors.CannotRenameFile,
			err,
		)
	}

	if oldFilename != newFilename {
		if err := os.Remove(oldPath); err != nil && os.IsNotExist(err) {
			return g_errors.WrapWithInfo(
				op,
				g_errors.CodeNotFound,
				g_errors.FileNotFound,
				map[string]any{
					"oldpath": oldPath,
				},
				err,
			)
		} else if err != nil {
			return g_errors.WrapWithInfo(
				op,
				g_errors.CodeInternal,
				g_errors.CannotDeleteFile,
				map[string]any{
					"oldpath": oldPath,
				},
				err,
			)
		}
	}

	return nil
}

// DownloadAndSaveImage fetches an image from a URL and saves it locally.
//
// The function performs HTTP GET request with a 5-second timeout, validates
// the response (status code, content type), downloads the image data, and
// saves it using SaveImage. The filename is automatically generated based
// on the URL and content type.
//
// Input parameters:
//   - url: HTTP/HTTPS URL pointing to an image; must not be empty
//
// Output parameters:
//   - string: generated filename of the saved image on success
//   - error: nil on success, or an error if URL is empty (CodeInvalidInput), HTTP request fails (timeout, network issues), response status is not 200 OK, Content-Type is not an image type (CodeInvalidInput), reading response body fails, or saving the image fails
func (u *Uploads) DownloadAndSaveImage(url string) (string, error) {
	const op = "storage.Uploads.DownloadAndSaveImage"
	if url == "" {
		return "", g_errors.NewWithInfo(
			op,
			g_errors.CodeInvalidInput,
			g_errors.InvalidImageURL,
			map[string]any{
				"url": url,
			},
		)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", g_errors.WrapWithInfo(
			op,
			g_errors.CodeInternal,
			g_errors.CannotDownloadImage,
			map[string]any{
				"url": url,
			},
			err,
		)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", g_errors.NewWithInfo(
			op,
			g_errors.CodeInternal,
			"",
			map[string]any{
				"respStatusCode": resp.StatusCode,
			},
		)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return "", g_errors.NewWithInfo(
			op,
			g_errors.CodeInvalidInput,
			g_errors.InvalidFileType,
			map[string]any{
				"info": g_errors.InvalidFileType,
				"url":  url,
				"type": contentType,
			},
		)
	}

	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", g_errors.WrapWithInfo(
			op,
			g_errors.CodeInternal,
			"",
			map[string]any{
				"url": url,
			},
			err,
		)
	}
	filename := u.GenerateImageFilename(url, contentType)

	if err := u.SaveImage(imageData, filename); err != nil {
		return "", err
	}

	return filename, nil
}

// ============================================================================
// HELPERS
// ============================================================================

// GenerateImageFilename creates a unique filename for an image based on its
// source URL and content type.
//
// The filename is generated by determining the appropriate file extension
// from Content-Type, creating a unique string combining timestamp and URL,
// computing SHA-256 hash and taking first 8 bytes as hex string.
//
// This approach ensures uniqueness (different URLs or upload times produce
// different names), determinism (same URL uploaded at same time produces
// same name), and safety (no special characters that could cause filesystem
// issues).
//
// Input parameters:
//   - url: source URL of the image (used for hash generation)
//   - contentType: HTTP Content-Type header (used for extension detection)
//
// Output parameters:
//   - string: generated filename with appropriate extension (e.g., "a1b2c3d4.jpg")
func (u *Uploads) GenerateImageFilename(url, contentType string) string {
	ext := ".jpg"
	switch {
	case strings.Contains(contentType, "png"):
		ext = ".png"
	case strings.Contains(contentType, "gif"):
		ext = ".gif"
	case strings.Contains(contentType, "webp"):
		ext = ".webp"
	}

	unique_string := time.Now().Format("20060102150405") + url

	hash := sha256.Sum256([]byte(unique_string))
	return fmt.Sprintf("%x%s", hash[:8], ext)
}

// ensureFolderExists verifies that the uploads directory exists and creates it
// if necessary.
//
// This method is called during initialization and is thread-safe.
// It creates the directory with 0755 permissions (owner: read/write/execute,
// group/others: read/execute).
//
// Output parameters:
//   - error: nil if directory exists or was created successfully, otherwise an error from os.MkdirAll
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
