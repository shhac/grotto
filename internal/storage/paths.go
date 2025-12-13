package storage

import (
	"os"
	"path/filepath"
)

const appName = ".grotto"

// DefaultStoragePath returns the default storage location for Grotto
// Platform-specific paths:
//   - macOS/Linux: ~/.grotto
//   - Windows: %USERPROFILE%\.grotto
func DefaultStoragePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, appName), nil
}
