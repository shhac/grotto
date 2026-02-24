package request

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2/data/binding"
	"github.com/shhac/grotto/internal/ui/form"
)

// ModeSynchronizer handles synchronization between Text and Form modes.
//
// ARCHITECTURE:
// This component solves the "freeze on mode switch" bug by centralizing all
// sync logic in one place with explicit state management.
//
// The freeze bug occurs when:
//  1. User switches mode (e.g., Text -> Form)
//  2. Mode binding changes, triggering listeners
//  3. Listener tries to sync data, which triggers more changes
//  4. Infinite loop causes UI freeze
//
// Solution:
//   - Atomic 'syncing' flag guards ALL sync operations (lock-free to avoid deadlocks)
//   - SwitchMode is the ONLY entry point for mode changes
//   - Sync functions are ONLY called from within SwitchMode (while syncing=true)
//   - External listeners check syncing flag before doing anything
//
// IMPORTANT: If you need to modify sync behavior, do it HERE, not in panel.go
type ModeSynchronizer struct {
	syncing  atomic.Bool       // Atomic flag for lock-free checking
	mu       sync.Mutex        // Protects builder and callback only
	mode     binding.String    // Current mode: "text" or "form"
	textData binding.String    // JSON text data
	builder  *form.FormBuilder // Form builder (may be nil)
	logger   *slog.Logger

	// Callbacks for external UI updates
	onModeChanged func(mode string) // Called AFTER sync completes
	onSyncError   func(err error)   // Called when text→form sync fails
}

// NewModeSynchronizer creates a new mode synchronizer
func NewModeSynchronizer(mode binding.String, textData binding.String, logger *slog.Logger) *ModeSynchronizer {
	return &ModeSynchronizer{
		mode:     mode,
		textData: textData,
		logger:   logger,
	}
}

// SetFormBuilder sets the form builder for sync operations
// Called when a new method is selected
func (s *ModeSynchronizer) SetFormBuilder(builder *form.FormBuilder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.builder = builder
}

// SetOnModeChanged sets callback for when mode changes complete
func (s *ModeSynchronizer) SetOnModeChanged(fn func(mode string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onModeChanged = fn
}

// SetOnSyncError sets callback for when text→form sync fails
func (s *ModeSynchronizer) SetOnSyncError(fn func(err error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSyncError = fn
}

// IsSyncing returns whether a sync operation is in progress
// External listeners should check this and return early if true
// Uses atomic load for lock-free checking (prevents deadlock with binding callbacks)
func (s *ModeSynchronizer) IsSyncing() bool {
	return s.syncing.Load()
}

// SwitchMode switches to the specified mode and syncs data.
// This is the ONLY entry point for mode changes.
//
// Flow:
//  1. Set syncing=true atomically (blocks external listeners)
//  2. Update mode binding
//  3. Sync data in appropriate direction
//  4. Set syncing=false
//  5. Call onModeChanged callback
func (s *ModeSynchronizer) SwitchMode(targetMode string) {
	// Atomic swap: if already syncing, return immediately
	if !s.syncing.CompareAndSwap(false, true) {
		return // Already syncing, ignore
	}

	// Use defer to ensure syncing is always reset
	defer func() {
		s.mu.Lock()
		callback := s.onModeChanged
		s.mu.Unlock()

		s.syncing.Store(false)

		// Call callback outside lock to avoid deadlock
		if callback != nil {
			callback(targetMode)
		}
	}()

	// Get current mode
	currentMode, _ := s.mode.Get()
	if currentMode == targetMode {
		return // No change needed
	}

	// Update mode binding (this may trigger listeners, but they'll check syncing)
	_ = s.mode.Set(targetMode)

	// Sync data based on direction
	if targetMode == "form" {
		s.syncTextToForm()
	} else if targetMode == "text" {
		s.syncFormToText()
	}
}

// GetMode returns the current mode
func (s *ModeSynchronizer) GetMode() string {
	mode, _ := s.mode.Get()
	return mode
}

// syncTextToForm parses text JSON and populates form
// INTERNAL: Only called from SwitchMode while syncing=true
func (s *ModeSynchronizer) syncTextToForm() {
	s.mu.Lock()
	builder := s.builder
	s.mu.Unlock()

	if builder == nil {
		return
	}

	textData, _ := s.textData.Get()
	if textData == "" {
		return
	}

	if err := builder.FromJSON(textData); err != nil {
		s.logger.Warn("failed to populate form from JSON", slog.Any("error", err))
		s.mu.Lock()
		cb := s.onSyncError
		s.mu.Unlock()
		if cb != nil {
			cb(err)
		}
	} else {
		// Clear any previous error on success
		s.mu.Lock()
		cb := s.onSyncError
		s.mu.Unlock()
		if cb != nil {
			cb(nil)
		}
	}
}

// syncFormToText converts form to JSON and updates text
// INTERNAL: Only called from SwitchMode while syncing=true
func (s *ModeSynchronizer) syncFormToText() {
	s.mu.Lock()
	builder := s.builder
	s.mu.Unlock()

	if builder == nil {
		return
	}

	jsonStr, err := builder.ToJSON()
	if err != nil {
		s.logger.Warn("failed to convert form to JSON", slog.Any("error", err))
		return
	}

	_ = s.textData.Set(jsonStr)
}

// SyncTextToFormNow syncs text to form immediately (for history load)
// Unlike SwitchMode, this doesn't change the mode
func (s *ModeSynchronizer) SyncTextToFormNow() {
	if !s.syncing.CompareAndSwap(false, true) {
		return
	}

	defer s.syncing.Store(false)

	s.syncTextToForm()
}

// SyncFormToTextNow syncs form to text immediately (for send operations)
// Unlike SwitchMode, this doesn't change the mode
func (s *ModeSynchronizer) SyncFormToTextNow() {
	// Atomic swap: if already syncing, return immediately
	if !s.syncing.CompareAndSwap(false, true) {
		return
	}

	defer s.syncing.Store(false)

	s.syncFormToText()
}
