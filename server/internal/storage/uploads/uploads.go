// internal/storage/uploads/uploads.go
package uploads

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	g_errors "games_webapp/internal/errors"
)

// ============================================================================
// TYPES
// ============================================================================

// defaultDownloadTimeout is used when NewUploads is called with a zero value
// (tests, ad-hoc callers). Production wires the real timeout from config.
const defaultDownloadTimeout = 5 * time.Second

// Uploads manages file operations for user-uploaded images.
//
// It provides thread-safe operations for saving, deleting, replacing, and
// downloading images to/from a designated folder on the filesystem.
type Uploads struct {
	// folderPath is the absolute or relative path to the directory where
	// uploaded images are stored. Always normalized via filepath.Clean and
	// ends with a path separator (required for safeJoin's prefix check).
	folderPath string

	// cleanRoot is folderPath without the trailing separator, cached for the
	// prefix comparison in safeJoin.
	cleanRoot string

	// mu serialises filesystem operations so Save / Delete / Replace on the
	// same filename cannot race. All operations are exclusive — a RWMutex
	// would pretend reads exist when they don't.
	mu sync.Mutex

	// httpClient is reused across all downloads so connections are pooled
	// instead of the keep-alive cache being thrown away per call.
	httpClient *http.Client
}

// ============================================================================
// INITIALIZATION
// ============================================================================

// NewUploads creates and initializes a new Uploads instance. downloadTimeout
// bounds a single cover download; pass 0 to fall back to defaultDownloadTimeout.
func NewUploads(folderPath string, downloadTimeout time.Duration) (*Uploads, error) {
	const op = "storage.NewUploads"
	if folderPath == "" {
		return nil, g_errors.NewWithInfo(op, g_errors.CodeInternal, "",
			map[string]any{"info": g_errors.EmptyFolderPath})
	}
	if downloadTimeout <= 0 {
		downloadTimeout = defaultDownloadTimeout
	}

	cleanRoot := filepath.Clean(folderPath)
	u := &Uploads{
		folderPath: cleanRoot + string(filepath.Separator),
		cleanRoot:  cleanRoot,
		httpClient: &http.Client{Timeout: downloadTimeout},
	}

	if err := u.ensureFolderExists(); err != nil {
		return nil, g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	return u, nil
}

// ============================================================================
// METHODS
// ============================================================================

// safeJoin returns an absolute filesystem path for `filename` rooted at
// u.folderPath, rejecting any filename that would escape the uploads directory
// (e.g. "../../etc/passwd"). The filename must be a simple file — no
// separators and no parent references — otherwise CodeInvalidInput is returned.
func (u *Uploads) safeJoin(op, filename string) (string, error) {
	if filename == "" {
		return "", g_errors.New(op, g_errors.CodeInvalidInput, g_errors.EmptyFileName)
	}
	// Reject separators and traversal outright: GenerateImageFilename never
	// produces them, and any caller that sneaks one in is almost certainly
	// attacker input.
	if strings.ContainsAny(filename, `/\`) || filename == "." || filename == ".." {
		return "", g_errors.NewWithInfo(op, g_errors.CodeInvalidInput, g_errors.EmptyFileName,
			map[string]any{"filename": filename})
	}
	full := filepath.Clean(filepath.Join(u.folderPath, filename))
	// Defense in depth: verify the cleaned path still lives inside the
	// uploads root. Covers exotic cases (Unicode normalization, odd Windows
	// prefixes) the string check above might not catch.
	if full != filepath.Join(u.cleanRoot, filename) {
		return "", g_errors.NewWithInfo(op, g_errors.CodeInvalidInput, g_errors.EmptyFileName,
			map[string]any{"filename": filename})
	}
	return full, nil
}

// SaveImage writes an image to the filesystem with the specified filename.
func (u *Uploads) SaveImage(image []byte, filename string) error {
	const op = "storage.Uploads.SaveImage"

	if len(image) == 0 {
		return g_errors.New(op, g_errors.CodeInvalidInput, g_errors.NullImageLength)
	}

	fullPath, err := u.safeJoin(op, filename)
	if err != nil {
		return err
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	if _, err := os.Stat(fullPath); err == nil {
		// Existing file = client conflict, not server fault: map to 409.
		return g_errors.NewWithInfo(op, g_errors.CodeConflict, g_errors.FileAlreadyExists,
			map[string]any{"fullPath": fullPath})
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return g_errors.Wrap(op, g_errors.CodeInternal, g_errors.CannotCreateFile, err)
	}
	defer file.Close()
	if _, err := file.Write(image); err != nil {
		_ = os.Remove(fullPath)
		return g_errors.Wrap(op, g_errors.CodeInternal, g_errors.CannotWriteFile, err)
	}

	return nil
}

// DeleteImage removes an image file from the filesystem.
func (u *Uploads) DeleteImage(filename string) error {
	const op = "storage.Uploads.DeleteImage"

	fullPath, err := u.safeJoin(op, filename)
	if err != nil {
		return err
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	if _, err := os.Stat(fullPath); err != nil {
		if os.IsNotExist(err) {
			return g_errors.WrapWithInfo(op, g_errors.CodeNotFound, g_errors.FileNotFound,
				map[string]any{"path": fullPath}, err)
		}
		return g_errors.WrapWithInfo(op, g_errors.CodeInternal, g_errors.CannotDeleteFile,
			map[string]any{"path": fullPath}, err)
	}

	if err := os.Remove(fullPath); err != nil {
		return g_errors.WrapWithInfo(op, g_errors.CodeInternal, g_errors.CannotDeleteFile,
			map[string]any{"path": fullPath}, err)
	}
	return nil
}

// ReplaceImage atomically replaces an existing image with new content and/or name.
func (u *Uploads) ReplaceImage(image []byte, oldFilename, newFilename string) error {
	const op = "storage.Uploads.ReplaceImage"

	if len(image) == 0 {
		return g_errors.New(op, g_errors.CodeInvalidInput, g_errors.NullImageLength)
	}

	oldPath, err := u.safeJoin(op, oldFilename)
	if err != nil {
		return err
	}
	newPath, err := u.safeJoin(op, newFilename)
	if err != nil {
		return err
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	if _, err := os.Stat(oldPath); oldFilename != newFilename && os.IsNotExist(err) {
		return g_errors.WrapWithInfo(op, g_errors.CodeNotFound, g_errors.FileNotFound,
			map[string]any{"oldfilename": oldFilename, "newfilename": newFilename}, err)
	} else if err != nil && !os.IsNotExist(err) {
		return g_errors.WrapWithInfo(op, g_errors.CodeInternal, g_errors.FileNotFound,
			map[string]any{"oldfilename": oldFilename, "newfilename": newFilename}, err)
	}

	if err := atomicWriteFile(newPath, image); err != nil {
		return g_errors.Wrap(op, g_errors.CodeInternal, "", err)
	}

	if oldFilename != newFilename {
		if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
			return g_errors.WrapWithInfo(op, g_errors.CodeInternal, g_errors.CannotDeleteFile,
				map[string]any{"oldpath": oldPath}, err)
		}
	}

	return nil
}

// DownloadAndSaveImage fetches an image from a URL and saves it locally.
//
// The body streams into a unique temp file in the uploads directory WITHOUT
// holding u.mu — a slow remote server must not block every other filesystem
// operation (Slowloris-style DoS). Only the final conflict-check + rename is
// serialised. Temp files are created via os.CreateTemp so concurrent workers
// can download independently without colliding.
func (u *Uploads) DownloadAndSaveImage(ctx context.Context, url string) (string, error) {
	const op = "storage.Uploads.DownloadAndSaveImage"
	if url == "" {
		return "", g_errors.NewWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidImageURL,
			map[string]any{"url": url})
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", g_errors.WrapWithInfo(op, g_errors.CodeInternal, g_errors.CannotDownloadImage,
			map[string]any{"url": url}, err)
	}
	resp, err := u.httpClient.Do(req)
	if err != nil {
		return "", g_errors.WrapWithInfo(op, g_errors.CodeInternal, g_errors.CannotDownloadImage,
			map[string]any{"url": url}, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", g_errors.NewWithInfo(op, g_errors.CodeInternal, "",
			map[string]any{"respStatusCode": resp.StatusCode})
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return "", g_errors.NewWithInfo(op, g_errors.CodeInvalidInput, g_errors.InvalidFileType,
			map[string]any{"url": url, "type": contentType})
	}

	filename := u.GenerateImageFilename(url, contentType)
	fullPath, err := u.safeJoin(op, filename)
	if err != nil {
		return "", err
	}

	// Download into an anonymous temp file in the uploads dir — same
	// filesystem as fullPath so the eventual rename is atomic.
	tmp, err := os.CreateTemp(u.cleanRoot, "dl_*.part")
	if err != nil {
		return "", g_errors.Wrap(op, g_errors.CodeInternal, g_errors.CannotCreateFile, err)
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", g_errors.Wrap(op, g_errors.CodeInternal, g_errors.CannotWriteFile, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", g_errors.Wrap(op, g_errors.CodeInternal, g_errors.CannotWriteFile, err)
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	if _, err := os.Stat(fullPath); err == nil {
		_ = os.Remove(tmpPath)
		return "", g_errors.NewWithInfo(op, g_errors.CodeConflict, g_errors.FileAlreadyExists,
			map[string]any{"fullPath": fullPath})
	}

	if err := os.Rename(tmpPath, fullPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", g_errors.Wrap(op, g_errors.CodeInternal, g_errors.CannotWriteFile, err)
	}

	return filename, nil
}

// ============================================================================
// HELPERS
// ============================================================================

// GenerateImageFilename creates a unique filename from source URL and
// content type. Output is `<8-byte hex>.<ext>` — no separators, safe for
// safeJoin.
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

	uniqueStr := time.Now().Format("20060102150405") + url
	hash := sha256.Sum256([]byte(uniqueStr))
	return fmt.Sprintf("%x%s", hash[:8], ext)
}

// atomicWriteFile writes data to a sibling ".tmp" file, closes it, then
// renames into place — either the target is fully replaced or it's unchanged.
func atomicWriteFile(path string, data []byte) error {
	tempPath := path + ".tmp"
	f, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tempPath)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return err
	}
	return nil
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
