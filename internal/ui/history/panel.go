package history

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/domain"
	"github.com/shhac/grotto/internal/storage"
)

// HistoryPanel displays request history with replay functionality
type HistoryPanel struct {
	widget.BaseWidget

	storage storage.Repository
	logger  *slog.Logger

	// UI components
	historyList binding.UntypedList
	listWidget  *widget.List
	clearButton *widget.Button
	statusLabel *widget.Label

	// Callbacks
	onReplay func(entry domain.HistoryEntry)

	// Content container
	content *fyne.Container
}

// NewHistoryPanel creates a new history panel
func NewHistoryPanel(storage storage.Repository, logger *slog.Logger) *HistoryPanel {
	p := &HistoryPanel{
		storage:     storage,
		logger:      logger,
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

			return container.NewBorder(
				nil,          // top
				nil,          // bottom
				nil,          // left
				replayButton, // right
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
			rightButton := border.Objects[1].(*widget.Button)
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
			rightButton.OnTapped = func() {
				if p.onReplay != nil {
					p.onReplay(historyEntry)
				}
			}
		},
	)

	// Header with status and clear button
	header := container.NewBorder(
		nil,           // top
		nil,           // bottom
		p.statusLabel, // left
		p.clearButton, // right
		nil,           // center
	)

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

// Refresh reloads history from storage
func (p *HistoryPanel) Refresh() {
	entries, err := p.storage.GetHistory(100)
	if err != nil {
		p.logger.Error("failed to load history", slog.Any("error", err))
		p.statusLabel.SetText("History (error)")
		return
	}

	// Convert to untyped list items
	items := make([]interface{}, len(entries))
	for i, entry := range entries {
		items[i] = entry
	}

	if err := p.historyList.Set(items); err != nil {
		p.logger.Error("failed to set history list", slog.Any("error", err))
		return
	}

	p.statusLabel.SetText(fmt.Sprintf("History (%d)", len(entries)))
	p.logger.Debug("history refreshed", slog.Int("count", len(entries)))
}

// SetOnReplay sets the callback when user clicks replay
func (p *HistoryPanel) SetOnReplay(fn func(entry domain.HistoryEntry)) {
	p.onReplay = fn
}

// handleClearAll clears all history
func (p *HistoryPanel) handleClearAll() {
	if err := p.storage.ClearHistory(); err != nil {
		p.logger.Error("failed to clear history", slog.Any("error", err))
		return
	}

	p.Refresh()
	p.logger.Info("history cleared")
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
