package history

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/domain"
	"github.com/shhac/grotto/internal/storage"
)

// HistoryPanel displays request history with replay functionality
type HistoryPanel struct {
	widget.BaseWidget

	storage storage.Repository
	logger  *slog.Logger
	window  fyne.Window

	// UI components
	historyList binding.UntypedList
	listWidget  *widget.List
	clearButton *widget.Button
	statusLabel *widget.Label

	// Filter state
	mu           sync.Mutex
	filterEntry  *widget.Entry
	filterQuery  string
	statusFilter string                // "" (all), "success", or "error"
	allEntries   []domain.HistoryEntry // full unfiltered entries from storage

	// Callbacks
	onReplay func(entry domain.HistoryEntry)
	onSelect func(entry domain.HistoryEntry)

	// Content container
	content *fyne.Container
}

// NewHistoryPanel creates a new history panel
func NewHistoryPanel(storage storage.Repository, logger *slog.Logger, window fyne.Window) *HistoryPanel {
	p := &HistoryPanel{
		storage:     storage,
		logger:      logger,
		window:      window,
		historyList: binding.NewUntypedList(),
	}

	p.ExtendBaseWidget(p)
	p.buildUI()
	p.Refresh()

	return p
}

// buildUI creates the panel UI
func (p *HistoryPanel) buildUI() {
	// Status label
	p.statusLabel = widget.NewLabel("History (0)")

	// Clear button
	p.clearButton = widget.NewButton("Clear All", func() {
		p.handleClearAll()
	})

	// Filter entry for searching history
	p.filterEntry = widget.NewEntry()
	p.filterEntry.SetPlaceHolder("Filter history...")
	p.filterEntry.OnChanged = func(query string) {
		p.filterQuery = strings.ToLower(query)
		p.applyFilter()
	}

	// Status filter dropdown
	statusSelect := widget.NewSelect([]string{"All", "Success", "Error"}, func(selected string) {
		switch selected {
		case "Success":
			p.statusFilter = "success"
		case "Error":
			p.statusFilter = "error"
		default:
			p.statusFilter = ""
		}
		p.applyFilter()
	})
	statusSelect.SetSelected("All")

	// History list
	p.listWidget = widget.NewListWithData(
		p.historyList,
		func() fyne.CanvasObject {
			// Template for list items
			timeLabel := widget.NewLabel("")
			methodLabel := widget.NewLabel("")
			methodLabel.TextStyle = fyne.TextStyle{Bold: true}
			statusLabel := widget.NewLabel("")
			durationLabel := widget.NewLabel("")
			replayButton := widget.NewButton("Replay", nil)
			deleteButton := widget.NewButtonWithIcon("", theme.DeleteIcon(), nil)

			return container.NewBorder(
				nil, // top
				nil, // bottom
				nil, // left
				container.NewHBox(replayButton, deleteButton), // right
				container.NewVBox(
					container.NewHBox(timeLabel, statusLabel, durationLabel),
					methodLabel,
				),
			)
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			// Update list item with data
			entry := item.(binding.Untyped)
			val, err := entry.Get()
			if err != nil {
				p.logger.Error("failed to get history entry", slog.Any("error", err))
				return
			}

			historyEntry, ok := val.(domain.HistoryEntry)
			if !ok {
				p.logger.Error("invalid history entry type")
				return
			}

			// Update UI elements
			border := obj.(*fyne.Container)
			rightBox := border.Objects[1].(*fyne.Container)
			replayButton := rightBox.Objects[0].(*widget.Button)
			deleteButton := rightBox.Objects[1].(*widget.Button)
			centerBox := border.Objects[0].(*fyne.Container)
			topRow := centerBox.Objects[0].(*fyne.Container)
			methodLabel := centerBox.Objects[1].(*widget.Label)

			timeLabel := topRow.Objects[0].(*widget.Label)
			statusLabel := topRow.Objects[1].(*widget.Label)
			durationLabel := topRow.Objects[2].(*widget.Label)

			// Format display
			timeLabel.SetText(historyEntry.Timestamp.Format("15:04:05"))
			methodLabel.SetText(p.formatMethodName(historyEntry.Method))
			durationLabel.SetText(fmt.Sprintf("%dms", historyEntry.Duration.Milliseconds()))

			// Status icon
			if historyEntry.Status == "success" {
				statusLabel.SetText("✓")
			} else {
				statusLabel.SetText("✗")
			}

			// Replay button
			replayButton.OnTapped = func() {
				if p.onReplay != nil {
					p.onReplay(historyEntry)
				}
			}

			// Delete button
			entryID := historyEntry.ID
			deleteButton.OnTapped = func() {
				if err := p.storage.DeleteHistoryEntry(entryID); err != nil {
					p.logger.Error("failed to delete history entry", slog.Any("error", err))
					return
				}
				p.Refresh()
			}
		},
	)

	// Click-to-load: tapping a row loads the entry into the UI
	p.listWidget.OnSelected = func(id widget.ListItemID) {
		if p.onSelect == nil {
			return
		}
		val, err := p.historyList.GetItem(id)
		if err != nil {
			p.logger.Error("failed to get history item on select", slog.Any("error", err))
			return
		}
		entry := val.(binding.Untyped)
		v, err := entry.Get()
		if err != nil {
			p.logger.Error("failed to get history entry on select", slog.Any("error", err))
			return
		}
		historyEntry, ok := v.(domain.HistoryEntry)
		if ok {
			p.onSelect(historyEntry)
		}
		// Deselect so the same item can be tapped again
		p.listWidget.UnselectAll()
	}

	// Header with status and clear button
	headerRow := container.NewBorder(
		nil,           // top
		nil,           // bottom
		p.statusLabel, // left
		p.clearButton, // right
		nil,           // center
	)

	// Filter row with text filter and status dropdown
	filterRow := container.NewBorder(
		nil, nil, nil,
		statusSelect,
		p.filterEntry,
	)

	header := container.NewVBox(headerRow, filterRow)

	// Build content
	p.content = container.NewBorder(
		header,       // top
		nil,          // bottom
		nil,          // left
		nil,          // right
		p.listWidget, // center
	)
}

// CreateRenderer implements the fyne.Widget interface
func (p *HistoryPanel) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.content)
}

// Refresh reloads history from storage and applies any active filter
func (p *HistoryPanel) Refresh() {
	entries, err := p.storage.GetHistory(100)
	if err != nil {
		p.logger.Error("failed to load history", slog.Any("error", err))
		fyne.Do(func() {
			p.statusLabel.SetText("History (error)")
		})
		return
	}

	p.mu.Lock()
	p.allEntries = entries
	p.mu.Unlock()
	p.applyFilter()
	p.logger.Debug("history refreshed", slog.Int("count", len(entries)))
}

// applyFilter filters allEntries by text query and status, then updates the list
func (p *HistoryPanel) applyFilter() {
	p.mu.Lock()
	entries := make([]domain.HistoryEntry, len(p.allEntries))
	copy(entries, p.allEntries)
	p.mu.Unlock()

	var filtered []domain.HistoryEntry
	for _, entry := range entries {
		// Status filter
		if p.statusFilter != "" && entry.Status != p.statusFilter {
			continue
		}
		// Text filter: match against method name, request body, error message
		if p.filterQuery != "" {
			method := strings.ToLower(entry.Method)
			request := strings.ToLower(entry.Request)
			errMsg := strings.ToLower(entry.Error)
			if !strings.Contains(method, p.filterQuery) &&
				!strings.Contains(request, p.filterQuery) &&
				!strings.Contains(errMsg, p.filterQuery) {
				continue
			}
		}
		filtered = append(filtered, entry)
	}

	items := make([]interface{}, len(filtered))
	for i, entry := range filtered {
		items[i] = entry
	}

	if err := p.historyList.Set(items); err != nil {
		p.logger.Error("failed to set history list", slog.Any("error", err))
		return
	}

	fyne.Do(func() {
		if p.filterQuery != "" || p.statusFilter != "" {
			p.statusLabel.SetText(fmt.Sprintf("History (%d of %d)", len(filtered), len(p.allEntries)))
		} else {
			p.statusLabel.SetText(fmt.Sprintf("History (%d)", len(p.allEntries)))
		}
	})
}

// SetOnSelect sets the callback when user clicks a history item (load without sending)
func (p *HistoryPanel) SetOnSelect(fn func(entry domain.HistoryEntry)) {
	p.onSelect = fn
}

// SetOnReplay sets the callback when user clicks replay
func (p *HistoryPanel) SetOnReplay(fn func(entry domain.HistoryEntry)) {
	p.onReplay = fn
}

// handleClearAll clears all history after user confirmation
func (p *HistoryPanel) handleClearAll() {
	dialog.ShowConfirm("Clear History",
		"Are you sure you want to clear all history entries?",
		func(confirmed bool) {
			if !confirmed {
				return
			}
			if err := p.storage.ClearHistory(); err != nil {
				p.logger.Error("failed to clear history", slog.Any("error", err))
				return
			}
			p.Refresh()
			p.logger.Info("history cleared")
		},
		p.window,
	)
}

// formatMethodName extracts and formats the method name for display
// Converts "package.Service/Method" to "Service.Method"
func (p *HistoryPanel) formatMethodName(fullMethod string) string {
	// Split on '/' to get service and method
	parts := strings.Split(fullMethod, "/")
	if len(parts) != 2 {
		return fullMethod
	}

	servicePath := parts[0]
	methodName := parts[1]

	// Get just the service name (last part after '.')
	serviceParts := strings.Split(servicePath, ".")
	serviceName := servicePath
	if len(serviceParts) > 0 {
		serviceName = serviceParts[len(serviceParts)-1]
	}

	return fmt.Sprintf("%s.%s", serviceName, methodName)
}

// AddEntry adds a new entry to history and refreshes the display
func (p *HistoryPanel) AddEntry(entry domain.HistoryEntry) error {
	if err := p.storage.AddHistoryEntry(entry); err != nil {
		p.logger.Error("failed to add history entry", slog.Any("error", err))
		return err
	}

	p.Refresh()
	return nil
}

// GenerateEntryID generates a unique ID for a history entry
func GenerateEntryID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// ClearHistory clears all history entries
func (p *HistoryPanel) ClearHistory() error {
	if err := p.storage.ClearHistory(); err != nil {
		return err
	}
	p.Refresh()
	return nil
}
