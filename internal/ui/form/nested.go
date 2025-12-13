package form

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// NestedMessageWidget displays a nested message as an expandable section
type NestedMessageWidget struct {
	widget.BaseWidget

	name      string
	md        protoreflect.MessageDescriptor
	expanded  bool
	builder   *FormBuilder // Nested form builder
	container fyne.CanvasObject
	accordion *widget.Accordion
}

// NewNestedMessageWidget creates an expandable nested message widget
func NewNestedMessageWidget(name string, md protoreflect.MessageDescriptor) *NestedMessageWidget {
	n := &NestedMessageWidget{
		name: name,
		md:   md,
	}

	// Create nested form builder
	n.builder = NewFormBuilder(md)

	// Create accordion for expand/collapse behavior
	n.accordion = widget.NewAccordion(
		widget.NewAccordionItem(
			name,
			n.builder.Build(),
		),
	)

	n.container = n.accordion
	n.ExtendBaseWidget(n)

	return n
}

// CreateRenderer implements fyne.Widget
func (n *NestedMessageWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(n.container)
}

// Toggle expands or collapses the nested message
func (n *NestedMessageWidget) Toggle() {
	n.expanded = !n.expanded
	if n.expanded {
		n.accordion.Open(0)
	} else {
		n.accordion.Close(0)
	}
}

// GetValue returns the nested message values as a map
func (n *NestedMessageWidget) GetValue() interface{} {
	if n.builder == nil {
		return nil
	}
	return n.builder.GetValues()
}

// SetValue populates nested fields from a map or protoreflect.Message
func (n *NestedMessageWidget) SetValue(v interface{}) {
	if n.builder == nil {
		return
	}

	switch val := v.(type) {
	case map[string]interface{}:
		n.builder.SetValues(val)
	case protoreflect.Message:
		// Convert protoreflect.Message to map
		values := make(map[string]interface{})
		val.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			values[string(fd.Name())] = v.Interface()
			return true
		})
		n.builder.SetValues(values)
	}
}

// SetExpanded programmatically sets the expansion state
func (n *NestedMessageWidget) SetExpanded(expanded bool) {
	n.expanded = expanded
	if expanded {
		n.accordion.Open(0)
	} else {
		n.accordion.Close(0)
	}
}

// GetBuilder returns the nested form builder for advanced access
func (n *NestedMessageWidget) GetBuilder() *FormBuilder {
	return n.builder
}
