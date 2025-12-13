package form

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// EnumWidget wraps a Select widget for enum fields
type EnumWidget struct {
	widget.BaseWidget

	name     string
	enum     protoreflect.EnumDescriptor
	selector *widget.Select
	values   []protoreflect.EnumValueDescriptor
}

// NewEnumWidget creates a new enum selector widget
func NewEnumWidget(name string, ed protoreflect.EnumDescriptor) *EnumWidget {
	w := &EnumWidget{
		name: name,
		enum: ed,
	}

	// Collect all enum values
	w.values = make([]protoreflect.EnumValueDescriptor, 0, ed.Values().Len())
	names := make([]string, 0, ed.Values().Len())

	for i := 0; i < ed.Values().Len(); i++ {
		evd := ed.Values().Get(i)
		w.values = append(w.values, evd)
		names = append(names, string(evd.Name()))
	}

	// Create selector with enum value names
	w.selector = widget.NewSelect(names, nil)

	// Set default to first value if available
	if len(names) > 0 {
		w.selector.SetSelected(names[0])
	}

	w.ExtendBaseWidget(w)
	return w
}

// GetValue returns the selected enum value number
func (e *EnumWidget) GetValue() int32 {
	selected := e.selector.Selected
	if selected == "" {
		return 0
	}

	// Find the enum value descriptor with matching name
	for _, evd := range e.values {
		if string(evd.Name()) == selected {
			return int32(evd.Number())
		}
	}

	return 0
}

// GetValueName returns the selected enum value name
func (e *EnumWidget) GetValueName() string {
	return e.selector.Selected
}

// SetValue sets by enum number
func (e *EnumWidget) SetValue(v int32) {
	// Find the enum value descriptor with matching number
	for _, evd := range e.values {
		if evd.Number() == protoreflect.EnumNumber(v) {
			e.selector.SetSelected(string(evd.Name()))
			return
		}
	}
}

// SetValueByName sets by enum name
func (e *EnumWidget) SetValueByName(name string) {
	// Check if this is a valid enum value
	for _, evd := range e.values {
		if string(evd.Name()) == name {
			e.selector.SetSelected(name)
			return
		}
	}
}

// CreateRenderer implements fyne.Widget
func (e *EnumWidget) CreateRenderer() fyne.WidgetRenderer {
	label := widget.NewLabel(e.name + ":")

	content := container.NewBorder(nil, nil, label, nil, e.selector)

	return widget.NewSimpleRenderer(content)
}
