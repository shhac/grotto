package browser

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/domain"
)

// ServiceBrowser displays services and methods in a tree view
type ServiceBrowser struct {
	widget.BaseWidget

	tree          *widget.Tree
	themedTree    fyne.CanvasObject // tree wrapped with custom theme
	services      binding.UntypedList // []domain.Service

	// Callbacks
	onMethodSelect func(service domain.Service, method domain.Method)
}

// NewServiceBrowser creates a new service browser widget
func NewServiceBrowser(services binding.UntypedList) *ServiceBrowser {
	b := &ServiceBrowser{
		services: services,
	}

	b.tree = widget.NewTree(
		b.childUIDs,
		b.isBranch,
		b.create,
		b.update,
	)

	b.tree.OnSelected = b.onTreeSelected

	// Create a themed container with custom chevron icons for the tree
	// This only affects the tree widget, not other UI elements
	customTheme := newTreeTheme(theme.DefaultTheme())
	b.themedTree = container.NewThemeOverride(b.tree, customTheme)

	b.ExtendBaseWidget(b)
	return b
}

// SetOnMethodSelect sets callback when a method is selected
func (b *ServiceBrowser) SetOnMethodSelect(fn func(service domain.Service, method domain.Method)) {
	b.onMethodSelect = fn
}

// Refresh updates the tree from the services binding
func (b *ServiceBrowser) Refresh() {
	b.tree.Refresh()
}

// CreateRenderer creates the renderer for this widget
func (b *ServiceBrowser) CreateRenderer() fyne.WidgetRenderer {
	// Return the themed container instead of the raw tree
	return widget.NewSimpleRenderer(b.themedTree)
}

// childUIDs returns the child UIDs for a given parent UID
func (b *ServiceBrowser) childUIDs(uid string) []string {
	if uid == "" {
		// Root level - return all services
		return b.getServiceUIDs()
	}

	// Check if this is a service (no colon means it's a service name)
	if !strings.Contains(uid, ":") {
		// Return methods for this service
		return b.getMethodUIDs(uid)
	}

	// Methods have no children
	return []string{}
}

// isBranch returns whether the given UID represents a branch node
func (b *ServiceBrowser) isBranch(uid string) bool {
	// Root level services are branches
	// Methods (containing ":") are leaves
	return !strings.Contains(uid, ":")
}

// create creates a new tree node widget
func (b *ServiceBrowser) create(branch bool) fyne.CanvasObject {
	// Both branches and leaves use same structure for consistency
	// (Fyne tree widget may have issues with inconsistent structures)
	icon := canvas.NewImageFromResource(theme.FolderIcon())
	icon.FillMode = canvas.ImageFillContain
	icon.SetMinSize(fyne.NewSize(16, 16))

	label := widget.NewLabel("")

	return container.NewHBox(icon, label)
}

// update updates a tree node widget with the appropriate data
func (b *ServiceBrowser) update(uid string, branch bool, obj fyne.CanvasObject) {
	cont := obj.(*fyne.Container)
	icon := cont.Objects[0].(*canvas.Image)
	label := cont.Objects[1].(*widget.Label)

	if branch {
		// Services: use a subtle folder icon to distinguish from methods
		// (tree widget already shows expand/collapse chevron)
		icon.Resource = theme.FolderIcon()
		icon.Refresh()

		// Count methods in this service
		service := b.findService(uid)
		methodCount := 0
		if service != nil {
			methodCount = len(service.Methods)
		}

		label.SetText(fmt.Sprintf("%s  (%d)", uid, methodCount))
		label.TextStyle = fyne.TextStyle{Bold: true}
		label.Importance = widget.MediumImportance
	} else {
		// Methods: show icon based on method type
		parts := strings.Split(uid, ":")
		if len(parts) == 2 {
			methodName := parts[1]
			service := b.findService(parts[0])
			if service != nil {
				method := b.findMethod(*service, methodName)
				if method != nil {
					// Set icon based on method type
					icon.Resource = b.getMethodIcon(method)
					icon.Refresh()

					// Format method name with subtle type badge
					typeBadge := b.getMethodTypeBadge(method)
					label.SetText(fmt.Sprintf("%s  %s", method.Name, typeBadge))
					label.TextStyle = fyne.TextStyle{}
					label.Importance = widget.MediumImportance
				}
			}
		}
	}
}

// getMethodIcon returns the appropriate icon for a method type
func (b *ServiceBrowser) getMethodIcon(method *domain.Method) fyne.Resource {
	if method.IsClientStream && method.IsServerStream {
		// Bidi stream - use media replay icon
		return theme.MediaReplayIcon()
	} else if method.IsServerStream {
		// Server stream - use download icon
		return theme.DownloadIcon()
	} else if method.IsClientStream {
		// Client stream - use upload icon
		return theme.UploadIcon()
	}
	// Unary - use mail send icon (distinct from tree chevrons)
	return theme.MailSendIcon()
}

// getMethodTypeBadge returns a subtle text badge for the method type
func (b *ServiceBrowser) getMethodTypeBadge(method *domain.Method) string {
	methodType := method.MethodType()
	switch methodType {
	case "Unary":
		return ""
	case "Client Stream":
		return "↑"
	case "Server Stream":
		return "↓"
	case "Bidi Stream":
		return "⇅"
	default:
		return ""
	}
}

// onTreeSelected handles tree selection events
func (b *ServiceBrowser) onTreeSelected(uid string) {
	if strings.Contains(uid, ":") {
		// Method selection (leaf)
		parts := strings.Split(uid, ":")
		if len(parts) == 2 {
			serviceName := parts[0]
			methodName := parts[1]

			service := b.findService(serviceName)
			if service != nil {
				method := b.findMethod(*service, methodName)
				if method != nil && b.onMethodSelect != nil {
					b.onMethodSelect(*service, *method)
				}
			}
		}
	} else {
		// Service selection (branch) - toggle expand/collapse
		if b.tree.IsBranchOpen(uid) {
			b.tree.CloseBranch(uid)
		} else {
			b.tree.OpenBranch(uid)
		}
		// Unselect so clicking the same service again will trigger OnSelected
		b.tree.UnselectAll()
	}
}

// getServiceUIDs returns the UIDs of all services
func (b *ServiceBrowser) getServiceUIDs() []string {
	serviceList, err := b.services.Get()
	if err != nil {
		return []string{}
	}

	var uids []string
	for _, item := range serviceList {
		if service, ok := item.(domain.Service); ok {
			uids = append(uids, service.FullName)
		}
	}
	return uids
}

// getMethodUIDs returns the UIDs of all methods for a given service
func (b *ServiceBrowser) getMethodUIDs(serviceName string) []string {
	service := b.findService(serviceName)
	if service == nil {
		return []string{}
	}

	var uids []string
	for _, method := range service.Methods {
		// Format: "service:method"
		uids = append(uids, fmt.Sprintf("%s:%s", serviceName, method.Name))
	}
	return uids
}

// findService finds a service by its full name
func (b *ServiceBrowser) findService(fullName string) *domain.Service {
	serviceList, err := b.services.Get()
	if err != nil {
		return nil
	}

	for _, item := range serviceList {
		if service, ok := item.(domain.Service); ok {
			if service.FullName == fullName {
				return &service
			}
		}
	}
	return nil
}

// findMethod finds a method by name within a service
func (b *ServiceBrowser) findMethod(service domain.Service, methodName string) *domain.Method {
	for _, method := range service.Methods {
		if method.Name == methodName {
			return &method
		}
	}
	return nil
}
