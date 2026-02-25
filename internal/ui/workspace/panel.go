package workspace

import (
	"log/slog"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/domain"
	"github.com/shhac/grotto/internal/storage"
)

// WorkspacePanel provides workspace management UI
type WorkspacePanel struct {
	widget.BaseWidget

	storage storage.Repository
	logger  *slog.Logger
	window  fyne.Window

	// UI components
	workspaceList binding.StringList
	listWidget    *widget.List
	nameEntry     *widget.Entry
	clearBtn      *widget.Button
	saveBtn       *widget.Button
	loadBtn       *widget.Button

	// Empty state
	placeholder *widget.Label

	// Callbacks
	onLoad func(workspace domain.Workspace)
	onSave func() domain.Workspace

	// Content container
	content *fyne.Container
}

// NewWorkspacePanel creates a new workspace management panel
func NewWorkspacePanel(storage storage.Repository, logger *slog.Logger, window fyne.Window) *WorkspacePanel {
	p := &WorkspacePanel{
		storage:       storage,
		logger:        logger,
		window:        window,
		workspaceList: binding.NewStringList(),
	}

	p.ExtendBaseWidget(p)
	p.buildUI()
	p.initializeComponents()
	p.RefreshList()

	return p
}

// buildUI constructs the workspace panel UI
func (p *WorkspacePanel) buildUI() {
	// Workspace list with per-item delete button
	p.listWidget = widget.NewListWithData(
		p.workspaceList,
		func() fyne.CanvasObject {
			label := widget.NewLabel("template")
			label.Truncation = fyne.TextTruncateEllipsis
			deleteBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), nil)
			deleteBtn.Importance = widget.LowImportance
			return container.NewBorder(nil, nil, nil, deleteBtn, label)
		},
		func(i binding.DataItem, o fyne.CanvasObject) {
			ct := o.(*fyne.Container)
			label := ct.Objects[0].(*widget.Label)
			deleteBtn := ct.Objects[1].(*widget.Button)

			strItem := i.(binding.String)
			val, _ := strItem.Get()
			label.SetText(val)

			deleteBtn.OnTapped = func() {
				p.handleDeleteWorkspace(val)
			}
		},
	)

	// Set selection handler
	p.listWidget.OnSelected = func(id widget.ListItemID) {
		items, _ := p.workspaceList.Get()
		if id >= 0 && id < len(items) {
			p.nameEntry.SetText(items[id])
		}
	}

	// Name entry with clear button
	p.nameEntry = widget.NewEntry()
	p.nameEntry.SetPlaceHolder("Workspace name")

	p.clearBtn = widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		p.nameEntry.SetText("")
		p.listWidget.UnselectAll()
	})
	p.clearBtn.Importance = widget.LowImportance

	// Empty state placeholder
	p.placeholder = widget.NewLabel("No saved workspaces — use Save to get started")
	p.placeholder.Alignment = fyne.TextAlignCenter
	p.placeholder.Wrapping = fyne.TextWrapWord
	p.placeholder.TextStyle = fyne.TextStyle{Italic: true}

	// Buttons
	p.saveBtn = widget.NewButton("Save", p.handleSave)
	p.loadBtn = widget.NewButton("Load", p.handleLoad)
}

// initializeComponents creates the layout once and stores it in p.content.
func (p *WorkspacePanel) initializeComponents() {
	// Title
	title := widget.NewLabelWithStyle("Workspaces", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// Name entry with clear button
	nameRow := container.NewBorder(nil, nil, nil, p.clearBtn, p.nameEntry)

	// Button row
	buttonRow := container.NewGridWithColumns(2,
		p.saveBtn,
		p.loadBtn,
	)

	// Main layout — stack placeholder over list for empty state
	p.content = container.NewBorder(
		title, // top
		container.NewVBox(nameRow, buttonRow), // bottom
		nil, // left
		nil, // right
		container.NewStack(container.NewScroll(p.listWidget), p.placeholder),
	)
}

// CreateRenderer implements the fyne.Widget interface
func (p *WorkspacePanel) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.content)
}

// RefreshList reloads workspace list from storage
func (p *WorkspacePanel) RefreshList() {
	workspaces, err := p.storage.ListWorkspaces()
	if err != nil {
		p.logger.Error("failed to list workspaces", slog.Any("error", err))
		return
	}

	if err := p.workspaceList.Set(workspaces); err != nil {
		p.logger.Error("failed to update workspace list", slog.Any("error", err))
	}

	if len(workspaces) == 0 {
		p.placeholder.Show()
	} else {
		p.placeholder.Hide()
	}
}

// SetOnLoad sets callback when workspace is loaded
func (p *WorkspacePanel) SetOnLoad(fn func(workspace domain.Workspace)) {
	p.onLoad = fn
}

// SetOnSave sets callback to get current state for saving
func (p *WorkspacePanel) SetOnSave(fn func() domain.Workspace) {
	p.onSave = fn
}

// TriggerSave programmatically triggers save (for keyboard shortcut)
func (p *WorkspacePanel) TriggerSave() {
	p.handleSave()
}

// TriggerLoad programmatically triggers load (for keyboard shortcut)
func (p *WorkspacePanel) TriggerLoad() {
	p.handleLoad()
}

// handleSave saves the current workspace
func (p *WorkspacePanel) handleSave() {
	name := p.nameEntry.Text
	if name == "" {
		ShowErrorDialog(p.window, "Please enter a workspace name")
		return
	}

	if p.onSave == nil {
		ShowErrorDialog(p.window, "Save handler not configured")
		return
	}

	// Get current state from callback
	workspace := p.onSave()
	workspace.Name = name

	doSave := func() {
		if err := p.storage.SaveWorkspace(workspace); err != nil {
			p.logger.Error("failed to save workspace",
				slog.String("name", name),
				slog.Any("error", err))
			ShowErrorDialog(p.window, "Failed to save workspace: "+err.Error())
			return
		}

		p.logger.Info("workspace saved", slog.String("name", name))
		ShowInfoDialog(p.window, "Workspace Saved", "Workspace '"+name+"' saved successfully")

		// Refresh list
		p.RefreshList()
	}

	// Check if workspace already exists and prompt for overwrite
	existing, _ := p.storage.ListWorkspaces()
	for _, w := range existing {
		if w == name {
			dialog.ShowConfirm("Overwrite Workspace",
				"A workspace named '"+name+"' already exists.\n\nSaving will replace it with your current connection, request, and method selection.",
				func(confirmed bool) {
					if confirmed {
						doSave()
					}
				},
				p.window,
			)
			return
		}
	}

	doSave()
}

// handleLoad loads the selected workspace
func (p *WorkspacePanel) handleLoad() {
	name := p.nameEntry.Text
	if name == "" {
		ShowErrorDialog(p.window, "Please select or enter a workspace name")
		return
	}

	if p.onLoad == nil {
		ShowErrorDialog(p.window, "Load handler not configured")
		return
	}

	// Load from storage
	workspace, err := p.storage.LoadWorkspace(name)
	if err != nil {
		p.logger.Error("failed to load workspace",
			slog.String("name", name),
			slog.Any("error", err))
		ShowErrorDialog(p.window, "Failed to load workspace: "+err.Error())
		return
	}

	p.logger.Info("workspace loaded", slog.String("name", name))

	// Apply via callback — no "loaded" dialog here because workspace
	// loading may trigger async connection. Success is evident from UI state.
	p.onLoad(*workspace)
}

// handleDeleteWorkspace deletes a workspace by name (called from per-item delete buttons)
func (p *WorkspacePanel) handleDeleteWorkspace(name string) {
	ShowDeleteConfirm(p.window, name, func() {
		if err := p.storage.DeleteWorkspace(name); err != nil {
			p.logger.Error("failed to delete workspace",
				slog.String("name", name),
				slog.Any("error", err))
			ShowErrorDialog(p.window, "Failed to delete workspace: "+err.Error())
			return
		}

		p.logger.Info("workspace deleted", slog.String("name", name))

		// Clear name entry if it matches the deleted workspace
		if p.nameEntry.Text == name {
			p.nameEntry.SetText("")
		}

		p.RefreshList()
	})
}
