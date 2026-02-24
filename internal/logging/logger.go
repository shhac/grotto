package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

const (
	// maxLogSize is the maximum log file size before rotation (5 MB).
	maxLogSize = 5 * 1024 * 1024
	// maxLogBackups is the number of rotated log files to keep.
	maxLogBackups = 3
)

// InitLogger initializes a structured logger with platform-specific log file paths.
// The logger writes JSON-formatted logs to a file in the appropriate platform location:
//   - macOS:   ~/Library/Logs/grotto/grotto.log
//   - Linux:   ~/.local/state/grotto/grotto.log
//   - Windows: %LOCALAPPDATA%\grotto\Logs\grotto.log
//
// When debug is true, the logger uses DEBUG level and includes source locations.
// Otherwise, it uses INFO level without source information.
func InitLogger(appName string, debug bool) (*slog.Logger, error) {
	logPath, err := getLogFilePath(appName)
	if err != nil {
		return nil, fmt.Errorf("failed to get log file path: %w", err)
	}

	// Create log directory if it doesn't exist
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory %s: %w", logDir, err)
	}

	// Rotate log file if it exceeds the size limit
	if err := rotateIfNeeded(logPath); err != nil {
		return nil, fmt.Errorf("failed to rotate log file: %w", err)
	}

	// Open log file for appending
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", logPath, err)
	}

	// Configure log level and options
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level:     level,
		AddSource: debug,
	})

	return slog.New(handler), nil
}

// rotateIfNeeded checks the log file size and rotates if it exceeds maxLogSize.
// Rotation renames current.log → current.log.1, .1 → .2, etc., keeping maxLogBackups.
func rotateIfNeeded(logPath string) error {
	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to rotate
		}
		return err
	}

	if info.Size() < maxLogSize {
		return nil
	}

	// Shift existing backups: .3 is deleted, .2→.3, .1→.2
	for i := maxLogBackups; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", logPath, i)
		dst := fmt.Sprintf("%s.%d", logPath, i+1)
		if i == maxLogBackups {
			os.Remove(src) // Delete oldest
		} else {
			os.Rename(src, dst) // Shift
		}
	}

	// Rotate current log to .1
	if err := os.Rename(logPath, logPath+".1"); err != nil {
		return fmt.Errorf("rotate log file: %w", err)
	}

	return nil
}

// getLogFilePath returns the platform-specific log file path.
// It uses runtime.GOOS to detect the current platform and constructs
// the appropriate path based on platform conventions.
func getLogFilePath(appName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	var logPath string
	switch runtime.GOOS {
	case "darwin": // macOS
		logPath = filepath.Join(homeDir, "Library", "Logs", appName, appName+".log")
	case "linux":
		logPath = filepath.Join(homeDir, ".local", "state", appName, appName+".log")
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			// Fallback if LOCALAPPDATA is not set
			localAppData = filepath.Join(homeDir, "AppData", "Local")
		}
		logPath = filepath.Join(localAppData, appName, "Logs", appName+".log")
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return logPath, nil
}

// NewNopLogger returns a no-op logger for testing.
// All log messages are discarded. This is useful for unit tests
// where logging output is not needed.
func NewNopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.NewFile(0, os.DevNull), &slog.HandlerOptions{
		Level: slog.LevelError + 1, // Higher than any log level, effectively disabling all logs
	}))
}
