package storage

import "github.com/shhac/grotto/internal/domain"

// Repository defines persistence operations for Grotto
type Repository interface {
	// Workspace operations
	SaveWorkspace(workspace domain.Workspace) error
	LoadWorkspace(name string) (*domain.Workspace, error)
	ListWorkspaces() ([]string, error)
	DeleteWorkspace(name string) error

	// Recent connections
	SaveRecentConnection(conn domain.Connection) error
	GetRecentConnections() ([]domain.Connection, error)
	ClearRecentConnections() error

	// History operations
	AddHistoryEntry(entry domain.HistoryEntry) error
	GetHistory(limit int) ([]domain.HistoryEntry, error)
	ClearHistory() error
}
