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

// mapSearchableEnumThreshold matches the threshold in mapper.go for consistency.
const mapSearchableEnumThreshold = 10

// MapFieldWidget displays a map with add/remove key-value pairs
type MapFieldWidget struct {
	widget.BaseWidget

	name      string
	fd        protoreflect.FieldDescriptor
	keyDesc   protoreflect.FieldDescriptor
	valueDesc protoreflect.FieldDescriptor
	items     []fyne.CanvasObject // List of key-value pair widgets
	container *fyne.Container
	listBox   *fyne.Container
	headerRow fyne.CanvasObject
	addButton *widget.Button

	onAdd    func()
	onRemove func(index int)
}

// NewMapFieldWidget creates a map widget for map fields
func NewMapFieldWidget(name string, fd protoreflect.FieldDescriptor) *MapFieldWidget {
	m := &MapFieldWidget{
		name:  name,
		fd:    fd,
		items: make([]fyne.CanvasObject, 0),
	}

	// Get key and value descriptors from map field
	m.keyDesc = fd.MapKey()
	m.valueDesc = fd.MapValue()

	// Create header row with "Key" / "Value" column labels
	keyLabel := widget.NewLabel("Key")
	keyLabel.TextStyle.Bold = true
	valLabel := widget.NewLabel("Value")
	valLabel.TextStyle.Bold = true
	headerGrid := container.NewGridWithColumns(2, keyLabel, valLabel)
	// Pad right side to match the space occupied by the remove button in data rows
	m.headerRow = container.NewBorder(nil, nil, nil, layout.NewSpacer(), headerGrid)

	// Create list container with header
	m.listBox = container.NewVBox(m.headerRow)

	// Create add button
	m.addButton = widget.NewButton("+ Add Entry", func() {
		m.AddEntry()
		if m.onAdd != nil {
			m.onAdd()
		}
	})

	// Main container with label, list, and add button.
	// Items grow naturally inside the VBox; the outer form VScroll handles scrolling.
	m.container = container.NewBorder(
		widget.NewLabel(name+":"),
		m.addButton,
		nil,
		nil,
		m.listBox,
	)

	m.ExtendBaseWidget(m)
	return m
}

// CreateRenderer implements fyne.Widget
func (m *MapFieldWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(m.container)
}

// AddEntry adds a new key-value pair to the map
func (m *MapFieldWidget) AddEntry() {
	// Create key widget
	keyWidget := m.createKeyWidget()

	// Create value widget
	valueWidget := m.createValueWidget()

	// Create row container — use nil center initially so we can reference
	// row in the remove button closure before finalizing the layout.
	row := container.NewBorder(nil, nil, nil, nil)

	// Create remove button with dynamic index lookup
	removeBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		currentIndex := -1
		for i, item := range m.items {
			if item == row {
				currentIndex = i
				break
			}
		}
		if currentIndex >= 0 {
			m.RemoveEntry(currentIndex)
			if m.onRemove != nil {
				m.onRemove(currentIndex)
			}
		}
	})

	// Set the row layout with key-value grid and remove button
	grid := container.NewGridWithColumns(2, keyWidget, valueWidget)
	row.Objects = []fyne.CanvasObject{grid, removeBtn}
	row.Layout = layout.NewBorderLayout(nil, nil, nil, removeBtn)

	m.items = append(m.items, row)
	m.listBox.Add(row)
	m.listBox.Refresh()
}

// RemoveEntry removes entry at the specified index
func (m *MapFieldWidget) RemoveEntry(index int) {
	if index < 0 || index >= len(m.items) {
		return
	}

	// Remove from items slice
	m.items = append(m.items[:index], m.items[index+1:]...)

	// Rebuild list box (header + data rows)
	m.rebuildListBox()
}

// GetValue returns a map of key-value pairs
func (m *MapFieldWidget) GetValue() interface{} {
	result := make(map[string]interface{})

	for _, item := range m.items {
		// Extract key and value from the row container
		if border, ok := item.(*fyne.Container); ok && len(border.Objects) > 0 {
			// The first object in border container is the grid with key and value
			if grid, ok := border.Objects[0].(*fyne.Container); ok && len(grid.Objects) >= 2 {
				keyWidget := grid.Objects[0]
				valueWidget := grid.Objects[1]

				// Extract key
				key := m.extractWidgetValue(keyWidget, m.keyDesc)
				keyStr, _ := key.(string) // Map keys are always strings in proto3

				// Extract value
				value := m.extractWidgetValue(valueWidget, m.valueDesc)

				// Only add non-empty keys
				if keyStr != "" {
					result[keyStr] = value
				}
			}
		}
	}

	return result
}

// SetValue populates the map from a map value
func (m *MapFieldWidget) SetValue(v interface{}) {
	// Clear existing items
	m.items = make([]fyne.CanvasObject, 0)
	m.rebuildListBox()

	// Populate from map
	if mapVal, ok := v.(map[string]interface{}); ok {
		for key, value := range mapVal {
			m.AddEntry()
			// Set values on the newly added entry
			if len(m.items) > 0 {
				lastItem := m.items[len(m.items)-1]
				if border, ok := lastItem.(*fyne.Container); ok && len(border.Objects) > 0 {
					if grid, ok := border.Objects[0].(*fyne.Container); ok && len(grid.Objects) >= 2 {
						keyWidget := grid.Objects[0]
						valueWidget := grid.Objects[1]

						// Set key
						m.setWidgetValue(keyWidget, key, m.keyDesc)

						// Set value
						m.setWidgetValue(valueWidget, value, m.valueDesc)
					}
				}
			}
		}
	}
}

// OnAdd sets a callback for when entries are added
func (m *MapFieldWidget) OnAdd(callback func()) {
	m.onAdd = callback
}

// OnRemove sets a callback for when entries are removed
func (m *MapFieldWidget) OnRemove(callback func(index int)) {
	m.onRemove = callback
}

// createKeyWidget creates a widget for the map key
// In proto3, map keys can only be integral or string types
func (m *MapFieldWidget) createKeyWidget() fyne.CanvasObject {
	switch m.keyDesc.Kind() {
	case protoreflect.StringKind:
		entry := newFormEntry()
		entry.SetPlaceHolder("key")
		return entry
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
	case protoreflect.BoolKind:
		return widget.NewCheck("", nil)
	default:
		return widget.NewLabel("Unsupported key type")
	}
}

// createValueWidget creates a widget for the map value
func (m *MapFieldWidget) createValueWidget() fyne.CanvasObject {
	switch m.valueDesc.Kind() {
	case protoreflect.BoolKind:
		return widget.NewCheck("", nil)
	case protoreflect.EnumKind:
		options := make([]string, 0)
		enumValues := m.valueDesc.Enum().Values()
		for i := 0; i < enumValues.Len(); i++ {
			options = append(options, string(enumValues.Get(i).Name()))
		}
		if len(options) > mapSearchableEnumThreshold {
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
		entry := newFormEntry()
		entry.SetPlaceHolder("value")
		return entry
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
	case protoreflect.MessageKind:
		nestedWidget := NewNestedMessageWidget(
			"Value",
			m.valueDesc.Message(),
		)
		return nestedWidget
	default:
		return widget.NewLabel("Unsupported value type")
	}
}

// extractWidgetValue extracts a value from a widget based on field descriptor
func (m *MapFieldWidget) extractWidgetValue(w fyne.CanvasObject, fd protoreflect.FieldDescriptor) interface{} {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		if check, ok := w.(*widget.Check); ok {
			return check.Checked
		}
	case protoreflect.StringKind:
		if entry, ok := w.(*widget.Entry); ok {
			return entry.Text
		}
	case protoreflect.EnumKind:
		if sel, ok := w.(*widget.Select); ok {
			enumValues := fd.Enum().Values()
			for i := 0; i < enumValues.Len(); i++ {
				val := enumValues.Get(i)
				if string(val.Name()) == sel.Selected {
					return int32(val.Number())
				}
			}
			return int32(0)
		}
		if selEntry, ok := w.(*widget.SelectEntry); ok {
			enumValues := fd.Enum().Values()
			for i := 0; i < enumValues.Len(); i++ {
				val := enumValues.Get(i)
				if string(val.Name()) == selEntry.Text {
					return int32(val.Number())
				}
			}
			return int32(0)
		}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		if entry, ok := w.(*widget.Entry); ok {
			if entry.Text == "" {
				return int32(0)
			}
			if val, err := parseScalarValue(entry.Text, fd); err == nil {
				return val
			}
		}
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		if entry, ok := w.(*widget.Entry); ok {
			if entry.Text == "" {
				return int64(0)
			}
			if val, err := parseScalarValue(entry.Text, fd); err == nil {
				return val
			}
		}
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		if entry, ok := w.(*widget.Entry); ok {
			if entry.Text == "" {
				return uint32(0)
			}
			if val, err := parseScalarValue(entry.Text, fd); err == nil {
				return val
			}
		}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		if entry, ok := w.(*widget.Entry); ok {
			if entry.Text == "" {
				return uint64(0)
			}
			if val, err := parseScalarValue(entry.Text, fd); err == nil {
				return val
			}
		}
	case protoreflect.FloatKind:
		if entry, ok := w.(*widget.Entry); ok {
			if entry.Text == "" {
				return float32(0)
			}
			if val, err := parseScalarValue(entry.Text, fd); err == nil {
				return val
			}
		}
	case protoreflect.DoubleKind:
		if entry, ok := w.(*widget.Entry); ok {
			if entry.Text == "" {
				return float64(0)
			}
			if val, err := parseScalarValue(entry.Text, fd); err == nil {
				return val
			}
		}
	case protoreflect.MessageKind:
		if nmw, ok := w.(*NestedMessageWidget); ok {
			return nmw.GetValue()
		}
	}

	return nil
}

// setWidgetValue sets a value on a widget based on field descriptor
func (m *MapFieldWidget) setWidgetValue(w fyne.CanvasObject, value interface{}, fd protoreflect.FieldDescriptor) {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		if check, ok := w.(*widget.Check); ok {
			if b, ok := value.(bool); ok {
				check.SetChecked(b)
			}
		}
	case protoreflect.StringKind:
		if entry, ok := w.(*widget.Entry); ok {
			if s, ok := value.(string); ok {
				entry.SetText(s)
			}
		}
	case protoreflect.EnumKind:
		var enumNum int32
		switch v := value.(type) {
		case int32:
			enumNum = v
		case float64:
			enumNum = int32(v)
		}
		enumValues := fd.Enum().Values()
		if sel, ok := w.(*widget.Select); ok {
			for i := 0; i < enumValues.Len(); i++ {
				val := enumValues.Get(i)
				if int32(val.Number()) == enumNum {
					sel.SetSelected(string(val.Name()))
					break
				}
			}
		} else if selEntry, ok := w.(*widget.SelectEntry); ok {
			for i := 0; i < enumValues.Len(); i++ {
				val := enumValues.Get(i)
				if int32(val.Number()) == enumNum {
					selEntry.SetText(string(val.Name()))
					break
				}
			}
		}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind,
		protoreflect.FloatKind, protoreflect.DoubleKind:
		if entry, ok := w.(*widget.Entry); ok {
			entry.SetText(fmt.Sprintf("%v", value))
		}
	case protoreflect.MessageKind:
		if nmw, ok := w.(*NestedMessageWidget); ok {
			nmw.SetValue(value)
		}
	}
}

// GetEntryCount returns the number of entries in the map
func (m *MapFieldWidget) GetEntryCount() int {
	return len(m.items)
}

// Clear removes all entries from the map
func (m *MapFieldWidget) Clear() {
	m.items = make([]fyne.CanvasObject, 0)
	m.rebuildListBox()
}

// rebuildListBox reconstructs the listBox with the header row followed by data rows.
func (m *MapFieldWidget) rebuildListBox() {
	objs := make([]fyne.CanvasObject, 0, 1+len(m.items))
	objs = append(objs, m.headerRow)
	objs = append(objs, m.items...)
	m.listBox.Objects = objs
	m.listBox.Refresh()
}
