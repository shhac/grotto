package form

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// oneofMember wraps a field inside a oneof with its widget and value accessors.
type oneofMember struct {
	widget   fyne.CanvasObject
	getValue func() interface{}
	setValue func(interface{})
}

// OneofWidget displays a selector for oneof field with conditional content
type OneofWidget struct {
	widget.BaseWidget

	name        string
	oneof       protoreflect.OneofDescriptor
	selector    *widget.Select
	fields      map[string]*oneofMember
	container   *fyne.Container
	activeField string
}

// NewOneofWidget creates a new oneof selector widget
func NewOneofWidget(name string, od protoreflect.OneofDescriptor) *OneofWidget {
	w := &OneofWidget{
		name:   name,
		oneof:  od,
		fields: make(map[string]*oneofMember),
	}

	// Collect all possible field names and create widgets
	fieldNames := make([]string, 0, od.Fields().Len())
	for i := 0; i < od.Fields().Len(); i++ {
		fd := od.Fields().Get(i)
		fieldName := string(fd.Name())
		fieldNames = append(fieldNames, fieldName)

		if fd.Kind() == protoreflect.MessageKind && !isWellKnownType(fd) {
			// Nested message: create a form builder with indented content
			builder := NewFormBuilder(fd.Message())
			leftPad := canvas.NewRectangle(color.Transparent)
			leftPad.SetMinSize(fyne.NewSize(12, 0))
			indented := container.NewBorder(nil, nil, leftPad, nil, builder.BuildContent())

			w.fields[fieldName] = &oneofMember{
				widget:   indented,
				getValue: func() interface{} { return builder.GetValues() },
				setValue: func(v interface{}) {
					if m, ok := v.(map[string]interface{}); ok {
						builder.SetValues(m)
					}
				},
			}
		} else {
			// Scalar, enum, or well-known type
			fieldWidget := MapFieldToWidget(fd)
			if fieldWidget != nil {
				w.fields[fieldName] = &oneofMember{
					widget:   fieldWidget.Widget,
					getValue: fieldWidget.GetValue,
					setValue: fieldWidget.SetValue,
				}
			}
		}
	}

	// Create selector without callback initially
	w.selector = widget.NewSelect(fieldNames, nil)

	// Set initial selection if available
	if len(fieldNames) > 0 {
		w.selector.SetSelected(fieldNames[0])
		w.activeField = fieldNames[0]
	}

	// Set callback after initial selection to avoid triggering during setup
	w.selector.OnChanged = func(selected string) {
		w.onFieldSelected(selected)
	}

	// Container for the active field widget
	w.container = container.NewVBox()
	if w.activeField != "" {
		if member, ok := w.fields[w.activeField]; ok {
			w.container.Objects = []fyne.CanvasObject{member.widget}
		}
	}

	w.ExtendBaseWidget(w)
	return w
}

// onFieldSelected handles field selection changes
func (o *OneofWidget) onFieldSelected(fieldName string) {
	if fieldName == o.activeField {
		return
	}

	o.activeField = fieldName

	// Update container to show only the selected field
	o.container.Objects = []fyne.CanvasObject{}
	if member, ok := o.fields[fieldName]; ok {
		o.container.Objects = []fyne.CanvasObject{member.widget}
	}
	o.container.Refresh()
}

// GetSelectedField returns which field is selected
func (o *OneofWidget) GetSelectedField() string {
	return o.activeField
}

// GetValue returns the value of the selected field as a map
func (o *OneofWidget) GetValue() interface{} {
	if o.activeField == "" {
		return nil
	}

	member, ok := o.fields[o.activeField]
	if !ok {
		return nil
	}

	// Return as a map with the field name as key
	return map[string]interface{}{
		o.activeField: member.getValue(),
	}
}

// SetValue sets the selected field and its value
func (o *OneofWidget) SetValue(fieldName string, value interface{}) {
	member, ok := o.fields[fieldName]
	if !ok {
		return
	}

	// Update selector
	o.selector.SetSelected(fieldName)

	// Set the field value
	member.setValue(value)
}

// CreateRenderer implements fyne.Widget
func (o *OneofWidget) CreateRenderer() fyne.WidgetRenderer {
	label := widget.NewLabel(o.name + ":")

	content := container.NewVBox(
		container.NewBorder(nil, nil, label, nil, o.selector),
		o.container,
	)

	return widget.NewSimpleRenderer(content)
}

// GetDescriptor returns the oneof descriptor
func (o *OneofWidget) GetDescriptor() protoreflect.OneofDescriptor {
	return o.oneof
}

// Clear resets the oneof to its default state
func (o *OneofWidget) Clear() {
	if len(o.fields) > 0 {
		// Get first field name
		fields := o.oneof.Fields()
		if fields.Len() > 0 {
			firstFieldName := string(fields.Get(0).Name())
			o.selector.SetSelected(firstFieldName)
			o.onFieldSelected(firstFieldName)
		}
	}
}
