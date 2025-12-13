package form

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// OneofWidget displays a selector for oneof field with conditional content
type OneofWidget struct {
	widget.BaseWidget

	name        string
	oneof       protoreflect.OneofDescriptor
	selector    *widget.Select
	fields      map[string]*FieldWidget // Field name -> widget
	container   *fyne.Container
	activeField string
}

// NewOneofWidget creates a new oneof selector widget
func NewOneofWidget(name string, od protoreflect.OneofDescriptor) *OneofWidget {
	w := &OneofWidget{
		name:   name,
		oneof:  od,
		fields: make(map[string]*FieldWidget),
	}

	// Collect all possible field names
	fieldNames := make([]string, 0, od.Fields().Len())
	for i := 0; i < od.Fields().Len(); i++ {
		fd := od.Fields().Get(i)
		fieldNames = append(fieldNames, string(fd.Name()))

		// Create widget for each possible field using mapper
		fieldWidget := MapFieldToWidget(fd)
		if fieldWidget != nil {
			w.fields[string(fd.Name())] = fieldWidget
		}
	}

	// Create selector
	w.selector = widget.NewSelect(fieldNames, func(selected string) {
		w.onFieldSelected(selected)
	})

	// Set initial selection if available
	if len(fieldNames) > 0 {
		w.selector.SetSelected(fieldNames[0])
		w.activeField = fieldNames[0]
	}

	// Container for the active field widget
	w.container = container.NewVBox()
	if w.activeField != "" {
		if fw, ok := w.fields[w.activeField]; ok {
			w.container.Objects = []fyne.CanvasObject{fw.Widget}
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
	if fieldWidget, ok := o.fields[fieldName]; ok {
		o.container.Objects = []fyne.CanvasObject{fieldWidget.Widget}
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

	fieldWidget, ok := o.fields[o.activeField]
	if !ok {
		return nil
	}

	// Return as a map with the field name as key
	return map[string]interface{}{
		o.activeField: fieldWidget.GetValue(),
	}
}

// SetValue sets the selected field and its value
func (o *OneofWidget) SetValue(fieldName string, value interface{}) {
	// Check if field exists
	fieldWidget, ok := o.fields[fieldName]
	if !ok {
		return
	}

	// Update selector
	o.selector.SetSelected(fieldName)

	// Set the field value
	fieldWidget.SetValue(value)
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
