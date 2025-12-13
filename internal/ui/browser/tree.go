package browser

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/domain"
)

// ServiceBrowser displays services and methods in a tree view
type ServiceBrowser struct {
	widget.BaseWidget

	tree     *widget.Tree
	services binding.UntypedList // []domain.Service

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
	return widget.NewSimpleRenderer(b.tree)
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
	return widget.NewLabel("")
}

// update updates a tree node widget with the appropriate data
func (b *ServiceBrowser) update(uid string, branch bool, obj fyne.CanvasObject) {
	label := obj.(*widget.Label)

	if branch {
		// This is a service
		label.SetText(uid)
		label.TextStyle = fyne.TextStyle{Bold: true}
		label.Importance = widget.MediumImportance
	} else {
		// This is a method - format as "MethodName (Type)"
		parts := strings.Split(uid, ":")
		if len(parts) == 2 {
			methodName := parts[1]
			service := b.findService(parts[0])
			if service != nil {
				method := b.findMethod(*service, methodName)
				if method != nil {
					label.SetText(fmt.Sprintf("%s (%s)", method.Name, method.MethodType()))
					label.Importance = widget.LowImportance
				}
			}
		}
	}
}

// onTreeSelected handles tree selection events
func (b *ServiceBrowser) onTreeSelected(uid string) {
	// Only handle method selections (leaves)
	if strings.Contains(uid, ":") {
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
