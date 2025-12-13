package form

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// RepeatedFieldWidget displays a list of values with add/remove functionality
type RepeatedFieldWidget struct {
	widget.BaseWidget

	name      string
	fd        protoreflect.FieldDescriptor
	items     []fyne.CanvasObject // List of item widgets
	container *fyne.Container
	listBox   *fyne.Container
	addButton *widget.Button

	onAdd    func()
	onRemove func(index int)
}

// NewRepeatedFieldWidget creates a list widget for repeated fields
func NewRepeatedFieldWidget(name string, fd protoreflect.FieldDescriptor) *RepeatedFieldWidget {
	r := &RepeatedFieldWidget{
		name:  name,
		fd:    fd,
		items: make([]fyne.CanvasObject, 0),
	}

	// Create list container
	r.listBox = container.NewVBox()

	// Create add button
	r.addButton = widget.NewButton("+ Add Item", func() {
		r.AddItem()
		if r.onAdd != nil {
			r.onAdd()
		}
	})

	// Main container with label, list, and add button
	r.container = container.NewBorder(
		widget.NewLabel(name+":"),
		r.addButton,
		nil,
		nil,
		container.NewVScroll(r.listBox),
	)

	r.ExtendBaseWidget(r)
	return r
}

// CreateRenderer implements fyne.Widget
func (r *RepeatedFieldWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.container)
}

// AddItem adds a new item to the list
func (r *RepeatedFieldWidget) AddItem() {
	index := len(r.items)

	// Create item widget based on field kind
	var itemWidget fyne.CanvasObject

	if r.fd.Kind() == protoreflect.MessageKind {
		// Repeated message: create nested form
		nestedWidget := NewNestedMessageWidget(
			fmt.Sprintf("Item %d", index+1),
			r.fd.Message(),
		)
		itemWidget = nestedWidget
	} else {
		// Repeated scalar: create appropriate input widget
		itemWidget = r.createScalarWidget()
	}

	// Create remove button
	removeBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		r.RemoveItem(index)
		if r.onRemove != nil {
			r.onRemove(index)
		}
	})

	// Create row with item and remove button
	row := container.NewBorder(
		nil,
		nil,
		nil,
		removeBtn,
		itemWidget,
	)

	r.items = append(r.items, row)
	r.listBox.Add(row)
	r.listBox.Refresh()
}

// RemoveItem removes item at the specified index
func (r *RepeatedFieldWidget) RemoveItem(index int) {
	if index < 0 || index >= len(r.items) {
		return
	}

	// Remove from items slice
	r.items = append(r.items[:index], r.items[index+1:]...)

	// Rebuild list box
	r.listBox.Objects = r.items
	r.listBox.Refresh()
}

// GetValue returns a slice of values from all items
func (r *RepeatedFieldWidget) GetValue() interface{} {
	values := make([]interface{}, 0, len(r.items))

	for _, item := range r.items {
		// Extract value from the row container
		if border, ok := item.(*fyne.Container); ok && len(border.Objects) > 0 {
			// The first object in border container is the actual widget
			w := border.Objects[0]

			// Extract values from widgets
			if nmw, ok := w.(*NestedMessageWidget); ok {
				values = append(values, nmw.GetValue())
			} else if entry, ok := w.(*widget.Entry); ok {
				values = append(values, entry.Text)
			} else if check, ok := w.(*widget.Check); ok {
				values = append(values, check.Checked)
			} else if sel, ok := w.(*widget.Select); ok {
				values = append(values, sel.Selected)
			}
		}
	}

	return values
}

// SetValue populates the list from a slice
func (r *RepeatedFieldWidget) SetValue(v interface{}) {
	// Clear existing items
	r.items = make([]fyne.CanvasObject, 0)
	r.listBox.Objects = nil
	r.listBox.Refresh()

	// Populate from slice
	if slice, ok := v.([]interface{}); ok {
		for _, item := range slice {
			r.AddItem()
			// Set value on the newly added item
			if len(r.items) > 0 {
				lastItem := r.items[len(r.items)-1]
				if border, ok := lastItem.(*fyne.Container); ok && len(border.Objects) > 0 {
					wid := border.Objects[0]

					if nmw, ok := wid.(*NestedMessageWidget); ok {
						nmw.SetValue(item)
					} else if entry, ok := wid.(*widget.Entry); ok {
						if str, ok := item.(string); ok {
							entry.SetText(str)
						}
					} else if check, ok := wid.(*widget.Check); ok {
						if b, ok := item.(bool); ok {
							check.SetChecked(b)
						}
					}
				}
			}
		}
	}
}

// OnAdd sets a callback for when items are added
func (r *RepeatedFieldWidget) OnAdd(callback func()) {
	r.onAdd = callback
}

// OnRemove sets a callback for when items are removed
func (r *RepeatedFieldWidget) OnRemove(callback func(index int)) {
	r.onRemove = callback
}

// createScalarWidget creates an appropriate widget for scalar repeated fields
func (r *RepeatedFieldWidget) createScalarWidget() fyne.CanvasObject {
	switch r.fd.Kind() {
	case protoreflect.BoolKind:
		return widget.NewCheck("", nil)
	case protoreflect.EnumKind:
		// Create select with enum values
		options := make([]string, 0)
		enumValues := r.fd.Enum().Values()
		for i := 0; i < enumValues.Len(); i++ {
			options = append(options, string(enumValues.Get(i).Name()))
		}
		return widget.NewSelect(options, nil)
	case protoreflect.Int32Kind, protoreflect.Int64Kind,
		protoreflect.Uint32Kind, protoreflect.Uint64Kind,
		protoreflect.Sint32Kind, protoreflect.Sint64Kind,
		protoreflect.Fixed32Kind, protoreflect.Fixed64Kind,
		protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind,
		protoreflect.FloatKind, protoreflect.DoubleKind:
		entry := widget.NewEntry()
		entry.SetPlaceHolder("0")
		return entry
	case protoreflect.StringKind:
		return widget.NewEntry()
	case protoreflect.BytesKind:
		entry := widget.NewEntry()
		entry.SetPlaceHolder("base64 or hex")
		return entry
	default:
		return widget.NewLabel("Unsupported type")
	}
}

// GetItemCount returns the number of items in the list
func (r *RepeatedFieldWidget) GetItemCount() int {
	return len(r.items)
}

// Clear removes all items from the list
func (r *RepeatedFieldWidget) Clear() {
	r.items = make([]fyne.CanvasObject, 0)
	r.listBox.Objects = nil
	r.listBox.Refresh()
}
