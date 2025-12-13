package storage

import (
	"fmt"
	"sync"

	"github.com/shhac/grotto/internal/domain"
)

// MemoryRepository implements Repository using in-memory storage for tests
type MemoryRepository struct {
	workspaces map[string]domain.Workspace
	recent     []domain.Connection
	history    []domain.HistoryEntry
	mu         sync.RWMutex
}

// NewMemoryRepository creates a new in-memory storage repository
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		workspaces: make(map[string]domain.Workspace),
		recent:     []domain.Connection{},
		history:    []domain.HistoryEntry{},
	}
}

// SaveWorkspace stores a workspace in memory
func (m *MemoryRepository) SaveWorkspace(workspace domain.Workspace) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.workspaces[workspace.Name] = workspace
	return nil
}

// LoadWorkspace retrieves a workspace from memory
func (m *MemoryRepository) LoadWorkspace(name string) (*domain.Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	workspace, ok := m.workspaces[name]
	if !ok {
		return nil, fmt.Errorf("workspace %q not found", name)
	}

	return &workspace, nil
}

// ListWorkspaces returns names of all stored workspaces
func (m *MemoryRepository) ListWorkspaces() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.workspaces))
	for name := range m.workspaces {
		names = append(names, name)
	}

	return names, nil
}

// DeleteWorkspace removes a workspace from memory
func (m *MemoryRepository) DeleteWorkspace(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.workspaces[name]; !ok {
		return fmt.Errorf("workspace %q not found", name)
	}

	delete(m.workspaces, name)
	return nil
}

// SaveRecentConnection adds a connection to recent list
func (m *MemoryRepository) SaveRecentConnection(conn domain.Connection) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove duplicate if exists
	m.recent = m.removeDuplicate(m.recent, conn)

	// Add to front
	m.recent = append([]domain.Connection{conn}, m.recent...)

	// Trim to max size
	if len(m.recent) > maxRecent {
		m.recent = m.recent[:maxRecent]
	}

	return nil
}

// GetRecentConnections returns the list of recent connections
func (m *MemoryRepository) GetRecentConnections() ([]domain.Connection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modification
	recent := make([]domain.Connection, len(m.recent))
	copy(recent, m.recent)

	return recent, nil
}

// ClearRecentConnections removes all recent connections
func (m *MemoryRepository) ClearRecentConnections() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recent = []domain.Connection{}
	return nil
}

// Helper methods

func (m *MemoryRepository) removeDuplicate(recent []domain.Connection, conn domain.Connection) []domain.Connection {
	var filtered []domain.Connection
	for _, r := range recent {
		if r.Address != conn.Address || r.UseTLS != conn.UseTLS {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// AddHistoryEntry adds a history entry to the history list
func (m *MemoryRepository) AddHistoryEntry(entry domain.HistoryEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Add to front (most recent first)
	m.history = append([]domain.HistoryEntry{entry}, m.history...)

	// Trim to max size
	if len(m.history) > maxHistory {
		m.history = m.history[:maxHistory]
	}

	return nil
}

// GetHistory returns the list of history entries, limited by the specified count
func (m *MemoryRepository) GetHistory(limit int) ([]domain.HistoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to prevent external modification
	history := make([]domain.HistoryEntry, len(m.history))
	copy(history, m.history)

	// Apply limit if specified and valid
	if limit > 0 && limit < len(history) {
		history = history[:limit]
	}

	return history, nil
}

// ClearHistory removes all history entries
func (m *MemoryRepository) ClearHistory() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.history = []domain.HistoryEntry{}
	return nil
}
