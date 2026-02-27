package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteStartupErrorFile_CreatesFileWithError(t *testing.T) {
	dir := t.TempDir()
	err := fmt.Errorf("failed to parse monitor config JSON: invalid character '}' looking for beginning of object key string")

	WriteStartupErrorFile(dir, err)

	data, readErr := os.ReadFile(filepath.Join(dir, "startup-error.log"))
	if readErr != nil {
		t.Fatalf("failed to read startup-error.log: %v", readErr)
	}

	content := string(data)

	// Should contain the error message
	if !strings.Contains(content, "invalid character '}'") {
		t.Errorf("expected error message in file, got:\n%s", content)
	}

	// Should contain a timestamp-like prefix
	if !strings.Contains(content, "STARTUP ERROR") {
		t.Errorf("expected STARTUP ERROR label in file, got:\n%s", content)
	}
}

func TestWriteStartupErrorFile_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "log", "ResourceAgent")
	err := fmt.Errorf("test error")

	WriteStartupErrorFile(dir, err)

	data, readErr := os.ReadFile(filepath.Join(dir, "startup-error.log"))
	if readErr != nil {
		t.Fatalf("directory not created or file not written: %v", readErr)
	}

	if !strings.Contains(string(data), "test error") {
		t.Errorf("expected error message, got: %s", string(data))
	}
}

func TestWriteStartupErrorFile_OverwritesPreviousFile(t *testing.T) {
	dir := t.TempDir()

	// Write first error
	WriteStartupErrorFile(dir, fmt.Errorf("first error"))
	// Write second error
	WriteStartupErrorFile(dir, fmt.Errorf("second error"))

	data, _ := os.ReadFile(filepath.Join(dir, "startup-error.log"))
	content := string(data)

	if strings.Contains(content, "first error") {
		t.Error("expected first error to be overwritten")
	}
	if !strings.Contains(content, "second error") {
		t.Errorf("expected second error in file, got: %s", content)
	}
}

func TestWriteStartupErrorFile_NilErrorSafe(t *testing.T) {
	dir := t.TempDir()

	// Should not panic with nil error
	WriteStartupErrorFile(dir, nil)

	_, readErr := os.ReadFile(filepath.Join(dir, "startup-error.log"))
	if readErr != nil {
		t.Fatalf("file should still be created: %v", readErr)
	}
}
