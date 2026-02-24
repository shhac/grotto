package browser

import (
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/domain"
)

// ServiceBrowser displays services and methods in a tree view
type ServiceBrowser struct {
	widget.BaseWidget

	tree        *widget.Tree
	services    binding.UntypedList // []domain.Service
	placeholder *widget.Label       // shown when no services loaded
	content     *fyne.Container     // stack switching between placeholder and tree

	// O(1) service lookup index, rebuilt when services binding changes
	serviceIndex map[string]domain.Service
	serviceUIDs  []string
	displayNames map[string]string // FullName → disambiguated short display name

	// Callbacks
	onMethodSelect func(service domain.Service, method domain.Method)
	onServiceError func(service domain.Service)
}

// NewServiceBrowser creates a new service browser widget
func NewServiceBrowser(services binding.UntypedList) *ServiceBrowser {
	b := &ServiceBrowser{
		services:     services,
		serviceIndex: make(map[string]domain.Service),
	}

	// Rebuild index when services change
	services.AddListener(binding.NewDataListener(func() {
		b.rebuildIndex()
	}))

	b.tree = widget.NewTree(
		b.childUIDs,
		b.isBranch,
		b.create,
		b.update,
	)

	b.tree.OnSelected = b.onTreeSelected

	// Empty state placeholder
	b.placeholder = widget.NewLabel("Enter a server address and click Connect to get started")
	b.placeholder.Alignment = fyne.TextAlignCenter
	b.placeholder.Wrapping = fyne.TextWrapWord
	b.placeholder.TextStyle = fyne.TextStyle{Italic: true}

	// Stack container: shows placeholder when empty, tree when populated
	// Use Border with spacers for vertical centering — NewCenter gives minimum width
	// which breaks word-wrapping labels (renders one char per line).
	placeholderCentered := container.NewBorder(nil, nil, nil, nil,
		container.NewVBox(layout.NewSpacer(), b.placeholder, layout.NewSpacer()),
	)
	b.content = container.NewStack(placeholderCentered)

	b.ExtendBaseWidget(b)
	return b
}

// SetOnMethodSelect sets callback when a method is selected
func (b *ServiceBrowser) SetOnMethodSelect(fn func(service domain.Service, method domain.Method)) {
	b.onMethodSelect = fn
}

// SetOnServiceError sets callback when an error service is selected
func (b *ServiceBrowser) SetOnServiceError(fn func(service domain.Service)) {
	b.onServiceError = fn
}

// Refresh updates the tree from the services binding
func (b *ServiceBrowser) Refresh() {
	b.tree.Refresh()
}

// SelectMethod programmatically opens a service branch and selects a method node.
// This triggers onTreeSelected which calls onMethodSelect.
func (b *ServiceBrowser) SelectMethod(serviceName, methodName string) {
	b.tree.OpenBranch(serviceName)
	uid := fmt.Sprintf("%s:%s", serviceName, methodName)
	b.tree.Select(uid)
}

// FocusTree moves keyboard focus to the service tree widget.
func (b *ServiceBrowser) FocusTree() {
	if c := fyne.CurrentApp().Driver().CanvasForObject(b.tree); c != nil {
		c.Focus(b.tree)
	}
}

// CreateRenderer creates the renderer for this widget
func (b *ServiceBrowser) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(b.content)
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
		service := b.findService(uid)

		displayName := b.displayNames[uid]
		if displayName == "" {
			displayName = uid
		}

		if service != nil && service.Error != "" {
			// Error service: show warning icon and indicator
			icon.Resource = theme.WarningIcon()
			icon.Refresh()
			label.SetText(fmt.Sprintf("%s  ⚠", displayName))
			label.TextStyle = fyne.TextStyle{Italic: true}
			label.Importance = widget.WarningImportance
		} else {
			// Normal service: show short name with method count
			icon.Resource = theme.FolderIcon()
			icon.Refresh()
			methodCount := 0
			if service != nil {
				methodCount = len(service.Methods)
			}
			label.SetText(fmt.Sprintf("%s  (%d)", displayName, methodCount))
			label.TextStyle = fyne.TextStyle{Bold: true}
			label.Importance = widget.MediumImportance
		}
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
	case "ClientStream":
		return "↑"
	case "ServerStream":
		return "↓"
	case "BidiStream":
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
		// Service selection (branch)
		service := b.findService(uid)
		if service != nil && service.Error != "" && b.onServiceError != nil {
			// Error service: show error details
			b.onServiceError(*service)
			b.tree.UnselectAll()
		} else {
			// Normal service: toggle expand/collapse
			if b.tree.IsBranchOpen(uid) {
				b.tree.CloseBranch(uid)
			} else {
				b.tree.OpenBranch(uid)
			}
			b.tree.UnselectAll()
		}
	}
}

// rebuildIndex rebuilds the O(1) service lookup index from the binding.
// Called via DataListener when the services binding changes.
func (b *ServiceBrowser) rebuildIndex() {
	serviceList, err := b.services.Get()
	if err != nil {
		b.serviceIndex = make(map[string]domain.Service)
		b.serviceUIDs = nil
		return
	}

	index := make(map[string]domain.Service, len(serviceList))
	uids := make([]string, 0, len(serviceList))
	for _, item := range serviceList {
		if service, ok := item.(domain.Service); ok {
			index[service.FullName] = service
			uids = append(uids, service.FullName)
		}
	}
	sort.Strings(uids)
	b.serviceIndex = index
	b.serviceUIDs = uids
	b.displayNames = buildDisplayNames(index)

	// Toggle between placeholder and tree based on service count
	// (content may be nil during initial construction)
	if b.content != nil {
		if len(uids) == 0 {
			b.content.Objects = []fyne.CanvasObject{
				container.NewBorder(nil, nil, nil, nil,
					container.NewVBox(layout.NewSpacer(), b.placeholder, layout.NewSpacer()),
				),
			}
		} else {
			b.content.Objects = []fyne.CanvasObject{b.tree}
		}
		b.content.Refresh()
	}
}

// getServiceUIDs returns the UIDs of all services
func (b *ServiceBrowser) getServiceUIDs() []string {
	return b.serviceUIDs
}

// getMethodUIDs returns the UIDs of all methods for a given service
func (b *ServiceBrowser) getMethodUIDs(serviceName string) []string {
	service := b.findService(serviceName)
	if service == nil {
		return []string{}
	}

	uids := make([]string, 0, len(service.Methods))
	for _, method := range service.Methods {
		// Format: "service:method"
		uids = append(uids, fmt.Sprintf("%s:%s", serviceName, method.Name))
	}
	sort.Strings(uids)
	return uids
}

// findService finds a service by its full name using the O(1) index
func (b *ServiceBrowser) findService(fullName string) *domain.Service {
	if service, ok := b.serviceIndex[fullName]; ok {
		return &service
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

// buildDisplayNames computes short display names for services, disambiguating
// collisions where multiple services share the same simple name.
// For example, if both "com.foo.UserService" and "com.bar.UserService" exist,
// they become "foo.UserService" and "bar.UserService" respectively.
func buildDisplayNames(index map[string]domain.Service) map[string]string {
	display := make(map[string]string, len(index))

	// Group full names by their simple Name (last segment)
	groups := make(map[string][]string) // simpleName → []fullName
	for fullName, svc := range index {
		groups[svc.Name] = append(groups[svc.Name], fullName)
	}

	for simpleName, fullNames := range groups {
		if len(fullNames) == 1 {
			// No collision — use the simple name
			display[fullNames[0]] = simpleName
			continue
		}
		// Collision — progressively add package segments until unique
		for _, fullName := range fullNames {
			segments := strings.Split(fullName, ".")
			// Walk backwards from the service name, adding segments until unique
			name := simpleName
			for i := len(segments) - 2; i >= 0; i-- {
				name = segments[i] + "." + name
				unique := true
				for _, other := range fullNames {
					if other != fullName && strings.HasSuffix(other, "."+name) {
						unique = false
						break
					}
				}
				if unique {
					break
				}
			}
			display[fullName] = name
		}
	}

	return display
}
