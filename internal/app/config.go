package app

import (
	"os"
	"strconv"
)

// Config holds application-wide configuration.
type Config struct {
	// Debug enables debug logging and additional diagnostics
	Debug bool

	// StoragePath is the directory where workspaces and settings are stored
	StoragePath string
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Debug:       false,
		StoragePath: "", // Will use DefaultStoragePath() from storage package
	}
}

// ConfigFromEnv creates a configuration from environment variables.
// Reads GROTTO_DEBUG to enable debug mode.
func ConfigFromEnv() *Config {
	cfg := DefaultConfig()

	// Check GROTTO_DEBUG environment variable
	if debugStr := os.Getenv("GROTTO_DEBUG"); debugStr != "" {
		if debug, err := strconv.ParseBool(debugStr); err == nil {
			cfg.Debug = debug
		}
	}

	// Check GROTTO_STORAGE_PATH environment variable
	if storagePath := os.Getenv("GROTTO_STORAGE_PATH"); storagePath != "" {
		cfg.StoragePath = storagePath
	}

	return cfg
}
