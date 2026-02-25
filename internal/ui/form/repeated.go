package form

import (
	"encoding/base64"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
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

	// Main container with label, list, and add button.
	// Items grow naturally inside the VBox; the outer form VScroll handles scrolling.
	r.container = container.NewBorder(
		fieldLabel(formatFieldLabel(name), repeatedTypeHint(fd)),
		r.addButton,
		nil,
		nil,
		r.listBox,
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
	itemNum := len(r.items) + 1

	// Create item widget based on field kind
	var itemWidget fyne.CanvasObject

	if r.fd.Kind() == protoreflect.MessageKind {
		// Repeated message: create nested form
		nestedWidget := NewNestedMessageWidget(
			fmt.Sprintf("Item %d", itemNum),
			r.fd.Message(),
		)
		itemWidget = nestedWidget
	} else {
		// Repeated scalar: create appropriate input widget
		itemWidget = r.createScalarWidget()
	}

	// Create row container first (before remove button callback)
	row := container.NewBorder(
		nil,
		nil,
		nil,
		nil, // Will set remove button after
		itemWidget,
	)

	// Create remove button with dynamic index lookup
	// Instead of capturing the index at creation time, find the row's current index when clicked
	removeBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		// Find the current index of this row
		currentIndex := -1
		for i, item := range r.items {
			if item == row {
				currentIndex = i
				break
			}
		}
		if currentIndex >= 0 {
			r.RemoveItem(currentIndex)
			if r.onRemove != nil {
				r.onRemove(currentIndex)
			}
		}
	})

	// Update the row to include the remove button
	row.Objects = []fyne.CanvasObject{itemWidget, removeBtn}
	row.Layout = layout.NewBorderLayout(nil, nil, nil, removeBtn)
	row.Refresh()

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
				// Parse value based on field kind
				val := r.parseEntryValue(entry.Text)
				values = append(values, val)
			} else if check, ok := w.(*widget.Check); ok {
				values = append(values, check.Checked)
			} else if sel, ok := w.(*widget.Select); ok {
				// Convert enum name to number for protobuf
				if r.fd.Kind() == protoreflect.EnumKind {
					enumValues := r.fd.Enum().Values()
					for i := 0; i < enumValues.Len(); i++ {
						ev := enumValues.Get(i)
						if string(ev.Name()) == sel.Selected {
							values = append(values, int32(ev.Number()))
							break
						}
					}
				} else {
					values = append(values, sel.Selected)
				}
			} else if selEntry, ok := w.(*widget.SelectEntry); ok {
				// Large enum: SelectEntry with type-to-filter
				if r.fd.Kind() == protoreflect.EnumKind {
					enumValues := r.fd.Enum().Values()
					for i := 0; i < enumValues.Len(); i++ {
						ev := enumValues.Get(i)
						if string(ev.Name()) == selEntry.Text {
							values = append(values, int32(ev.Number()))
							break
						}
					}
				} else {
					values = append(values, selEntry.Text)
				}
			}
		}
	}

	return values
}

// parseEntryValue parses the entry text based on the field kind
func (r *RepeatedFieldWidget) parseEntryValue(text string) interface{} {
	switch r.fd.Kind() {
	case protoreflect.StringKind:
		return text
	case protoreflect.BytesKind:
		return text // Keep as string, will be converted later
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		if val, err := parseScalarValue(text, r.fd); err == nil {
			return val
		}
		return int32(0)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		if val, err := parseScalarValue(text, r.fd); err == nil {
			return val
		}
		return int64(0)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		if val, err := parseScalarValue(text, r.fd); err == nil {
			return val
		}
		return uint32(0)
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		if val, err := parseScalarValue(text, r.fd); err == nil {
			return val
		}
		return uint64(0)
	case protoreflect.FloatKind:
		if val, err := parseScalarValue(text, r.fd); err == nil {
			return val
		}
		return float32(0)
	case protoreflect.DoubleKind:
		if val, err := parseScalarValue(text, r.fd); err == nil {
			return val
		}
		return float64(0)
	default:
		return text
	}
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
						// Handle both string and numeric values
						entry.SetText(fmt.Sprintf("%v", item))
					} else if check, ok := wid.(*widget.Check); ok {
						if b, ok := item.(bool); ok {
							check.SetChecked(b)
						}
					} else if sel, ok := wid.(*widget.Select); ok {
						// Handle enum values (could be string name or int value)
						if str, ok := item.(string); ok {
							sel.SetSelected(str)
						} else if num, ok := item.(float64); ok {
							// JSON numbers come as float64 - convert to enum name
							enumValues := r.fd.Enum().Values()
							enumNum := int32(num)
							for i := 0; i < enumValues.Len(); i++ {
								ev := enumValues.Get(i)
								if int32(ev.Number()) == enumNum {
									sel.SetSelected(string(ev.Name()))
									break
								}
							}
						}
					} else if selEntry, ok := wid.(*widget.SelectEntry); ok {
						// Large enum: SelectEntry
						if str, ok := item.(string); ok {
							selEntry.SetText(str)
						} else if num, ok := item.(float64); ok {
							enumValues := r.fd.Enum().Values()
							enumNum := int32(num)
							for i := 0; i < enumValues.Len(); i++ {
								ev := enumValues.Get(i)
								if int32(ev.Number()) == enumNum {
									selEntry.SetText(string(ev.Name()))
									break
								}
							}
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

// searchableEnumThreshold matches the threshold in mapper.go for consistency.
const repeatedSearchableEnumThreshold = 10

// createScalarWidget creates an appropriate widget for scalar repeated fields
func (r *RepeatedFieldWidget) createScalarWidget() fyne.CanvasObject {
	switch r.fd.Kind() {
	case protoreflect.BoolKind:
		return widget.NewCheck("", nil)
	case protoreflect.EnumKind:
		options := make([]string, 0)
		enumValues := r.fd.Enum().Values()
		for i := 0; i < enumValues.Len(); i++ {
			options = append(options, string(enumValues.Get(i).Name()))
		}
		if len(options) > repeatedSearchableEnumThreshold {
			selEntry := widget.NewSelectEntry(options)
			selEntry.Wrapping = fyne.TextWrapOff
			selEntry.Scroll = container.ScrollNone
			selEntry.SetPlaceHolder("Type to filter...")
			allOptions := options
			selEntry.OnChanged = func(text string) {
				if text == "" {
					selEntry.SetOptions(allOptions)
					return
				}
				lower := strings.ToLower(text)
				filtered := make([]string, 0)
				for _, opt := range allOptions {
					if strings.Contains(strings.ToLower(opt), lower) {
						filtered = append(filtered, opt)
					}
				}
				selEntry.SetOptions(filtered)
			}
			selEntry.Validator = func(s string) error {
				if s == "" {
					return nil
				}
				for i := 0; i < enumValues.Len(); i++ {
					if string(enumValues.Get(i).Name()) == s {
						return nil
					}
				}
				return fmt.Errorf("unknown enum value: %s", s)
			}
			if len(options) > 0 {
				selEntry.SetText(options[0])
			}
			return selEntry
		}
		sel := widget.NewSelect(options, nil)
		if len(options) > 0 {
			sel.SetSelected(options[0])
		}
		return sel
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		entry := newFormEntry()
		entry.SetPlaceHolder("0")
		entry.Validator = func(s string) error {
			if s == "" {
				return nil
			}
			return ValidateInt32(s)
		}
		return entry
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		entry := newFormEntry()
		entry.SetPlaceHolder("0")
		entry.Validator = func(s string) error {
			if s == "" {
				return nil
			}
			return ValidateInt64(s)
		}
		return entry
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		entry := newFormEntry()
		entry.SetPlaceHolder("0")
		entry.Validator = func(s string) error {
			if s == "" {
				return nil
			}
			return ValidateUint32(s)
		}
		return entry
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		entry := newFormEntry()
		entry.SetPlaceHolder("0")
		entry.Validator = func(s string) error {
			if s == "" {
				return nil
			}
			return ValidateUint64(s)
		}
		return entry
	case protoreflect.FloatKind:
		entry := newFormEntry()
		entry.SetPlaceHolder("0.0")
		entry.Validator = func(s string) error {
			if s == "" {
				return nil
			}
			return ValidateFloat(s)
		}
		return entry
	case protoreflect.DoubleKind:
		entry := newFormEntry()
		entry.SetPlaceHolder("0.0")
		entry.Validator = func(s string) error {
			if s == "" {
				return nil
			}
			return ValidateDouble(s)
		}
		return entry
	case protoreflect.StringKind:
		return newFormEntry()
	case protoreflect.BytesKind:
		entry := newFormEntry()
		entry.SetPlaceHolder("Base64 encoded bytes")
		entry.Validator = func(s string) error {
			if s == "" {
				return nil
			}
			_, err := base64.StdEncoding.DecodeString(s)
			return err
		}
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
