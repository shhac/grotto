package workspace

import (
	"log/slog"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
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
	saveBtn       *widget.Button
	loadBtn       *widget.Button
	deleteBtn     *widget.Button
	newBtn        *widget.Button

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
	// Workspace list
	p.listWidget = widget.NewListWithData(
		p.workspaceList,
		func() fyne.CanvasObject {
			return widget.NewLabel("template")
		},
		func(i binding.DataItem, o fyne.CanvasObject) {
			label := o.(*widget.Label)
			strItem := i.(binding.String)
			val, _ := strItem.Get()
			label.SetText(val)
		},
	)

	// Set selection handler
	p.listWidget.OnSelected = func(id widget.ListItemID) {
		items, _ := p.workspaceList.Get()
		if id >= 0 && id < len(items) {
			p.nameEntry.SetText(items[id])
		}
	}

	// Name entry
	p.nameEntry = widget.NewEntry()
	p.nameEntry.SetPlaceHolder("Workspace name")

	// Empty state placeholder
	p.placeholder = widget.NewLabel("No saved workspaces — use Save Current above")
	p.placeholder.Alignment = fyne.TextAlignCenter
	p.placeholder.Wrapping = fyne.TextWrapWord
	p.placeholder.TextStyle = fyne.TextStyle{Italic: true}

	// Buttons
	p.saveBtn = widget.NewButton("Save Current", p.handleSave)
	p.loadBtn = widget.NewButton("Load", p.handleLoad)
	p.deleteBtn = widget.NewButton("Delete", p.handleDelete)
	p.deleteBtn.Importance = widget.DangerImportance
	p.newBtn = widget.NewButton("New", p.handleNew)
}

// initializeComponents creates the layout once and stores it in p.content.
func (p *WorkspacePanel) initializeComponents() {
	// Title
	title := widget.NewLabelWithStyle("Workspaces", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// Button row
	buttonRow := container.NewGridWithColumns(2,
		p.saveBtn,
		p.loadBtn,
	)

	actionRow := container.NewGridWithColumns(2,
		p.deleteBtn,
		p.newBtn,
	)

	// Main layout — stack placeholder over list for empty state
	p.content = container.NewBorder(
		title, // top
		container.NewVBox(p.nameEntry, buttonRow, actionRow), // bottom
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
				"Workspace '"+name+"' already exists. Overwrite it?",
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

// handleDelete deletes the selected workspace
func (p *WorkspacePanel) handleDelete() {
	name := p.nameEntry.Text
	if name == "" {
		ShowErrorDialog(p.window, "Please select or enter a workspace name")
		return
	}

	// Show confirmation
	ShowDeleteConfirm(p.window, name, func() {
		if err := p.storage.DeleteWorkspace(name); err != nil {
			p.logger.Error("failed to delete workspace",
				slog.String("name", name),
				slog.Any("error", err))
			ShowErrorDialog(p.window, "Failed to delete workspace: "+err.Error())
			return
		}

		p.logger.Info("workspace deleted", slog.String("name", name))

		// Clear name entry
		p.nameEntry.SetText("")

		// Refresh list
		p.RefreshList()

		ShowInfoDialog(p.window, "Workspace Deleted", "Workspace '"+name+"' deleted successfully")
	})
}

// handleNew clears the name entry for a new workspace
func (p *WorkspacePanel) handleNew() {
	p.nameEntry.SetText("")
	p.listWidget.UnselectAll()
}
