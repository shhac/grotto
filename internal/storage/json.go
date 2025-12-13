package storage

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/shhac/grotto/internal/domain"
)

const (
	workspacesDir  = "workspaces"
	recentFile     = "recent.json"
	historyFile    = "history.json"
	maxRecent      = 10
	maxHistory     = 100
	filePermission = 0644
	dirPermission  = 0755
)

// JSONRepository implements Repository using JSON files
type JSONRepository struct {
	basePath string
	logger   *slog.Logger
}

// NewJSONRepository creates a new JSON-based storage repository
func NewJSONRepository(basePath string, logger *slog.Logger) *JSONRepository {
	return &JSONRepository{
		basePath: basePath,
		logger:   logger,
	}
}

// SaveWorkspace saves a workspace to a JSON file
func (r *JSONRepository) SaveWorkspace(workspace domain.Workspace) error {
	if err := r.ensureWorkspacesDir(); err != nil {
		return fmt.Errorf("ensure workspaces directory: %w", err)
	}

	path := r.workspacePath(workspace.Name)
	data, err := json.MarshalIndent(workspace, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal workspace: %w", err)
	}

	if err := os.WriteFile(path, data, filePermission); err != nil {
		return fmt.Errorf("write workspace file: %w", err)
	}

	r.logger.Debug("saved workspace",
		slog.String("name", workspace.Name),
		slog.String("path", path))

	return nil
}

// LoadWorkspace loads a workspace from a JSON file
func (r *JSONRepository) LoadWorkspace(name string) (*domain.Workspace, error) {
	path := r.workspacePath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("workspace %q not found", name)
		}
		return nil, fmt.Errorf("read workspace file: %w", err)
	}

	var workspace domain.Workspace
	if err := json.Unmarshal(data, &workspace); err != nil {
		return nil, fmt.Errorf("unmarshal workspace: %w", err)
	}

	r.logger.Debug("loaded workspace",
		slog.String("name", name),
		slog.String("path", path))

	return &workspace, nil
}

// ListWorkspaces returns names of all saved workspaces
func (r *JSONRepository) ListWorkspaces() ([]string, error) {
	workspacesPath := filepath.Join(r.basePath, workspacesDir)

	// If directory doesn't exist, return empty list (not an error)
	if _, err := os.Stat(workspacesPath); os.IsNotExist(err) {
		r.logger.Debug("workspaces directory does not exist, returning empty list")
		return []string{}, nil
	}

	entries, err := os.ReadDir(workspacesPath)
	if err != nil {
		return nil, fmt.Errorf("read workspaces directory: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".json" {
			// Remove .json extension
			name := entry.Name()[:len(entry.Name())-5]
			names = append(names, name)
		}
	}

	r.logger.Debug("listed workspaces", slog.Int("count", len(names)))
	return names, nil
}

// DeleteWorkspace removes a workspace file
func (r *JSONRepository) DeleteWorkspace(name string) error {
	path := r.workspacePath(name)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("workspace %q not found", name)
		}
		return fmt.Errorf("delete workspace file: %w", err)
	}

	r.logger.Debug("deleted workspace",
		slog.String("name", name),
		slog.String("path", path))

	return nil
}

// SaveRecentConnection adds a connection to recent list
func (r *JSONRepository) SaveRecentConnection(conn domain.Connection) error {
	if err := r.ensureBaseDir(); err != nil {
		return fmt.Errorf("ensure base directory: %w", err)
	}

	recent, err := r.loadRecentList()
	if err != nil {
		return fmt.Errorf("load recent connections: %w", err)
	}

	// Remove duplicate if exists
	recent = r.removeDuplicate(recent, conn)

	// Add to front
	recent = append([]domain.Connection{conn}, recent...)

	// Trim to max size
	if len(recent) > maxRecent {
		recent = recent[:maxRecent]
	}

	if err := r.saveRecentList(recent); err != nil {
		return fmt.Errorf("save recent connections: %w", err)
	}

	r.logger.Debug("saved recent connection",
		slog.String("address", conn.Address))

	return nil
}

// GetRecentConnections returns the list of recent connections
func (r *JSONRepository) GetRecentConnections() ([]domain.Connection, error) {
	recent, err := r.loadRecentList()
	if err != nil {
		return nil, fmt.Errorf("load recent connections: %w", err)
	}

	r.logger.Debug("loaded recent connections", slog.Int("count", len(recent)))
	return recent, nil
}

// ClearRecentConnections removes all recent connections
func (r *JSONRepository) ClearRecentConnections() error {
	path := r.recentPath()
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			// Already clear, not an error
			return nil
		}
		return fmt.Errorf("delete recent connections file: %w", err)
	}

	r.logger.Debug("cleared recent connections")
	return nil
}

// Helper methods

func (r *JSONRepository) ensureBaseDir() error {
	if err := os.MkdirAll(r.basePath, dirPermission); err != nil {
		return fmt.Errorf("create base directory: %w", err)
	}
	return nil
}

func (r *JSONRepository) ensureWorkspacesDir() error {
	path := filepath.Join(r.basePath, workspacesDir)
	if err := os.MkdirAll(path, dirPermission); err != nil {
		return fmt.Errorf("create workspaces directory: %w", err)
	}
	return nil
}

func (r *JSONRepository) workspacePath(name string) string {
	return filepath.Join(r.basePath, workspacesDir, name+".json")
}

func (r *JSONRepository) recentPath() string {
	return filepath.Join(r.basePath, recentFile)
}

func (r *JSONRepository) loadRecentList() ([]domain.Connection, error) {
	path := r.recentPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, return empty list
			return []domain.Connection{}, nil
		}
		return nil, fmt.Errorf("read recent file: %w", err)
	}

	var recent []domain.Connection
	if err := json.Unmarshal(data, &recent); err != nil {
		return nil, fmt.Errorf("unmarshal recent connections: %w", err)
	}

	return recent, nil
}

func (r *JSONRepository) saveRecentList(recent []domain.Connection) error {
	data, err := json.MarshalIndent(recent, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal recent connections: %w", err)
	}

	path := r.recentPath()
	if err := os.WriteFile(path, data, filePermission); err != nil {
		return fmt.Errorf("write recent file: %w", err)
	}

	return nil
}

func (r *JSONRepository) removeDuplicate(recent []domain.Connection, conn domain.Connection) []domain.Connection {
	var filtered []domain.Connection
	for _, r := range recent {
		if r.Address != conn.Address || r.UseTLS != conn.UseTLS {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// AddHistoryEntry adds a history entry to the history list
func (r *JSONRepository) AddHistoryEntry(entry domain.HistoryEntry) error {
	if err := r.ensureBaseDir(); err != nil {
		return fmt.Errorf("ensure base directory: %w", err)
	}

	history, err := r.loadHistoryList()
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}

	// Add to front (most recent first)
	history = append([]domain.HistoryEntry{entry}, history...)

	// Trim to max size
	if len(history) > maxHistory {
		history = history[:maxHistory]
	}

	if err := r.saveHistoryList(history); err != nil {
		return fmt.Errorf("save history: %w", err)
	}

	r.logger.Debug("saved history entry",
		slog.String("id", entry.ID),
		slog.String("method", entry.Method))

	return nil
}

// GetHistory returns the list of history entries, limited by the specified count
func (r *JSONRepository) GetHistory(limit int) ([]domain.HistoryEntry, error) {
	history, err := r.loadHistoryList()
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}

	// Apply limit if specified and valid
	if limit > 0 && limit < len(history) {
		history = history[:limit]
	}

	r.logger.Debug("loaded history", slog.Int("count", len(history)))
	return history, nil
}

// ClearHistory removes all history entries
func (r *JSONRepository) ClearHistory() error {
	path := r.historyPath()
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			// Already clear, not an error
			return nil
		}
		return fmt.Errorf("delete history file: %w", err)
	}

	r.logger.Debug("cleared history")
	return nil
}

// historyPath returns the path to the history file
func (r *JSONRepository) historyPath() string {
	return filepath.Join(r.basePath, historyFile)
}

// loadHistoryList loads the history list from disk
func (r *JSONRepository) loadHistoryList() ([]domain.HistoryEntry, error) {
	path := r.historyPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, return empty list
			return []domain.HistoryEntry{}, nil
		}
		return nil, fmt.Errorf("read history file: %w", err)
	}

	var history []domain.HistoryEntry
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("unmarshal history: %w", err)
	}

	return history, nil
}

// saveHistoryList saves the history list to disk
func (r *JSONRepository) saveHistoryList(history []domain.HistoryEntry) error {
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal history: %w", err)
	}

	path := r.historyPath()
	if err := os.WriteFile(path, data, filePermission); err != nil {
		return fmt.Errorf("write history file: %w", err)
	}

	return nil
}
