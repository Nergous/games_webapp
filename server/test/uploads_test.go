package test

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"games_webapp/internal/storage/uploads"
)

func TestNewUploads(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "uploads_test")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tempDir)

		u, err := uploads.NewUploads(tempDir)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if u == nil {
			t.Error("expected Uploads instance, got nil")
		}
	})
	t.Run("empty folder path", func(t *testing.T) {
		u, err := uploads.NewUploads("")
		if err == nil {
			t.Error("expected error for empty path, got nil")
		}
		if u != nil {
			t.Error("expected nil Uploads for empty path")
		}
	})

	t.Run("nonexistent folder creation", func(t *testing.T) {
		tempDir := filepath.Join(os.TempDir(), "nonexistent_subdir", "uploads_test")
		defer os.RemoveAll(tempDir)

		u, err := uploads.NewUploads(tempDir)
		if err != nil {
			t.Errorf("expected folder to be created, got error: %v", err)
		}
		if u == nil {
			t.Error("expected Uploads instance, got nil")
		}

		// Verify folder was created
		if _, err := os.Stat(tempDir); os.IsNotExist(err) {
			t.Error("folder was not created")
		}
	})
}

func TestSaveImage(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "uploads_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	u, err := uploads.NewUploads(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	testImage := []byte("test image data")

	t.Run("successful save", func(t *testing.T) {
		filename := "test1.jpg"
		err := u.SaveImage(testImage, filename)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Verify file was created
		fullPath := filepath.Join(tempDir, filename)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Error("file was not created")
		}

		// Verify file content
		content, err := os.ReadFile(fullPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(content, testImage) {
			t.Error("file content does not match original image")
		}
	})

	t.Run("empty image data", func(t *testing.T) {
		err := u.SaveImage([]byte{}, "empty.jpg")
		if err != uploads.ErrInvalidImage {
			t.Errorf("expected ErrInvalidImage, got %v", err)
		}
	})

	t.Run("empty filename", func(t *testing.T) {
		err := u.SaveImage(testImage, "")
		if err != uploads.ErrInvalidFileName {
			t.Errorf("expected ErrInvalidFileName, got %v", err)
		}
	})

	t.Run("duplicate filename", func(t *testing.T) {
		filename := "duplicate.jpg"
		err := u.SaveImage(testImage, filename)
		if err != nil {
			t.Fatal(err)
		}

		err = u.SaveImage(testImage, filename)
		if err != uploads.ErrFileExists {
			t.Errorf("expected ErrFileExists, got %v", err)
		}
	})
}

func TestDeleteImage(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "uploads_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	u, err := uploads.NewUploads(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	testImage := []byte("test image data")
	filename := "to_delete.jpg"

	// Prepare test file
	err = u.SaveImage(testImage, filename)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("successful delete", func(t *testing.T) {
		err := u.DeleteImage(filename)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Verify file was deleted
		fullPath := filepath.Join(tempDir, filename)
		if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
			t.Error("file was not deleted")
		}
	})

	t.Run("delete non-existent file", func(t *testing.T) {
		err := u.DeleteImage("nonexistent.jpg")
		if err != uploads.ErrFileNotExists {
			t.Errorf("expected ErrFileNotExists, got %v", err)
		}
	})

	t.Run("empty filename", func(t *testing.T) {
		err := u.DeleteImage("")
		if err != uploads.ErrInvalidFileName {
			t.Errorf("expected ErrInvalidFileName, got %v", err)
		}
	})
}

func TestReplaceImage(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "uploads_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	u, err := uploads.NewUploads(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	oldImage := []byte("old image data")
	newImage := []byte("new image data")
	filename := "to_replace.jpg"

	// Prepare test file
	err = u.SaveImage(oldImage, filename)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("successful replace", func(t *testing.T) {
		err := u.ReplaceImage(newImage, filename)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		// Verify file content was replaced
		fullPath := filepath.Join(tempDir, filename)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(content, newImage) {
			t.Error("file content was not replaced")
		}
	})

	t.Run("replace with empty image", func(t *testing.T) {
		err := u.ReplaceImage([]byte{}, filename)
		if err != uploads.ErrInvalidImage {
			t.Errorf("expected ErrInvalidImage, got %v", err)
		}
	})

	t.Run("empty filename", func(t *testing.T) {
		err := u.ReplaceImage(newImage, "")
		if err != uploads.ErrInvalidFileName {
			t.Errorf("expected ErrInvalidFileName, got %v", err)
		}
	})

	t.Run("replace non-existent file", func(t *testing.T) {
		err := u.ReplaceImage(newImage, "nonexistent.jpg")
		if err == nil {
			t.Error("expected error when replacing non-existent file, got nil")
		}
	})
}

func TestConcurrentAccess(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "uploads_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	u, err := uploads.NewUploads(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	testImage := []byte("test image data")
	filename := "concurrent.jpg"

	// Test concurrent saves
	t.Run("concurrent saves", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make(chan error, 10)

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := u.SaveImage(testImage, filename)
				if err != nil && err != uploads.ErrFileExists {
					errors <- err
				}
			}()
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			t.Errorf("unexpected error during concurrent save: %v", err)
		}

		// Verify only one file was created
		files, err := filepath.Glob(filepath.Join(tempDir, filename))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Errorf("expected 1 file, got %d", len(files))
		}
	})

	// Test concurrent delete and replace
	t.Run("concurrent delete and replace", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make(chan error, 2)

		wg.Add(2)
		go func() {
			defer wg.Done()
			err := u.DeleteImage(filename)
			if err != nil {
				errors <- err
			}
		}()

		go func() {
			defer wg.Done()
			err := u.ReplaceImage(testImage, filename)
			if err != nil {
				errors <- err
			}
		}()

		wg.Wait()
		close(errors)

		for err := range errors {
			// We expect one of the operations to fail due to race condition,
			// but it shouldn't be any unexpected error
			if err != uploads.ErrFileNotExists {
				t.Errorf("unexpected error during concurrent operations: %v", err)
			}
		}
	})
}
