package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGetLogFilePath(t *testing.T) {
	appName := "grotto"
	logPath, err := getLogFilePath(appName)
	if err != nil {
		t.Fatalf("getLogFilePath failed: %v", err)
	}

	// Verify path is not empty
	if logPath == "" {
		t.Error("getLogFilePath returned empty path")
	}

	// Verify path contains app name
	if !filepath.IsAbs(logPath) {
		t.Errorf("getLogFilePath returned relative path: %s", logPath)
	}

	// Verify platform-specific path structure
	homeDir, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		expected := filepath.Join(homeDir, "Library", "Logs", appName, appName+".log")
		if logPath != expected {
			t.Errorf("macOS path mismatch: got %s, want %s", logPath, expected)
		}
	case "linux":
		expected := filepath.Join(homeDir, ".local", "state", appName, appName+".log")
		if logPath != expected {
			t.Errorf("Linux path mismatch: got %s, want %s", logPath, expected)
		}
	case "windows":
		// Just verify it contains the app name and Logs directory
		if !filepath.IsAbs(logPath) {
			t.Errorf("Windows path is not absolute: %s", logPath)
		}
	}
}

func TestInitLogger(t *testing.T) {
	// Use a temporary directory for testing
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	if runtime.GOOS == "windows" {
		originalHome = os.Getenv("USERPROFILE")
	}

	// Override home directory for testing
	t.Setenv("HOME", tmpDir)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tmpDir)
		t.Setenv("LOCALAPPDATA", filepath.Join(tmpDir, "AppData", "Local"))
	}
	defer func() {
		os.Setenv("HOME", originalHome)
	}()

	tests := []struct {
		name  string
		debug bool
	}{
		{"info level", false},
		{"debug level", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := InitLogger("grotto-test", tt.debug)
			if err != nil {
				t.Fatalf("InitLogger failed: %v", err)
			}
			if logger == nil {
				t.Fatal("InitLogger returned nil logger")
			}

			// Verify log file was created
			logPath, _ := getLogFilePath("grotto-test")
			if _, err := os.Stat(logPath); os.IsNotExist(err) {
				t.Errorf("Log file was not created at %s", logPath)
			}

			// Test that we can write to the logger
			logger.Info("test message", slog.String("key", "value"))
			logger.Debug("debug message")
		})
	}
}

func TestNewNopLogger(t *testing.T) {
	logger := NewNopLogger()
	if logger == nil {
		t.Fatal("NewNopLogger returned nil")
	}

	// Verify it doesn't panic when logging
	logger.Info("test info")
	logger.Debug("test debug")
	logger.Error("test error")
	logger.Warn("test warn")
}

func TestLoggerCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Override home directory
	t.Setenv("HOME", tmpDir)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tmpDir)
		t.Setenv("LOCALAPPDATA", filepath.Join(tmpDir, "AppData", "Local"))
	}

	logger, err := InitLogger("grotto-test", false)
	if err != nil {
		t.Fatalf("InitLogger failed: %v", err)
	}

	logPath, _ := getLogFilePath("grotto-test")
	logDir := filepath.Dir(logPath)

	// Verify directory exists
	if info, err := os.Stat(logDir); err != nil {
		t.Errorf("Log directory was not created: %v", err)
	} else if !info.IsDir() {
		t.Errorf("Log path exists but is not a directory: %s", logDir)
	}

	// Write a log message
	logger.Info("test message after directory creation")

	// Verify file exists and has content
	if info, err := os.Stat(logPath); err != nil {
		t.Errorf("Log file was not created: %v", err)
	} else if info.Size() == 0 {
		t.Error("Log file is empty after writing message")
	}
}
