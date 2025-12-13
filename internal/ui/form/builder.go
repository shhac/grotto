package form

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// FormBuilder generates Fyne forms from proto message descriptors
type FormBuilder struct {
	md             protoreflect.MessageDescriptor
	fields         map[string]*FieldWidget // Scalar field widgets
	repeatedFields map[string]*RepeatedFieldWidget
	nestedFields   map[string]*NestedMessageWidget
	oneofFields    map[string]*OneofWidget
	container      *fyne.Container
}

// NewFormBuilder creates a new form builder for a message descriptor
func NewFormBuilder(md protoreflect.MessageDescriptor) *FormBuilder {
	return &FormBuilder{
		md:             md,
		fields:         make(map[string]*FieldWidget),
		repeatedFields: make(map[string]*RepeatedFieldWidget),
		nestedFields:   make(map[string]*NestedMessageWidget),
		oneofFields:    make(map[string]*OneofWidget),
	}
}

// Build creates the form UI for the message descriptor
func (b *FormBuilder) Build() fyne.CanvasObject {
	items := make([]fyne.CanvasObject, 0)

	// Iterate through all fields in the message
	fields := b.md.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		fieldName := string(fd.Name())

		// Skip fields that are part of a oneof (handled separately)
		if fd.ContainingOneof() != nil {
			continue
		}

		// Handle different field types
		if fd.IsList() {
			// Repeated field
			repeatedWidget := NewRepeatedFieldWidget(fieldName, fd)
			b.repeatedFields[fieldName] = repeatedWidget
			items = append(items, repeatedWidget)

		} else if fd.IsMap() {
			// Map field - create a specialized map widget
			mapLabel := widget.NewLabel(fmt.Sprintf("%s (map): Not yet implemented", fieldName))
			items = append(items, mapLabel)

		} else if fd.Kind() == protoreflect.MessageKind {
			// Check if it's a well-known type
			if isWellKnownType(fd) {
				// Well-known types are handled by MapFieldToWidget
				fw := MapFieldToWidget(fd)
				if fw != nil {
					b.fields[fieldName] = fw
					// Create form item with label and widget
					formItem := container.NewBorder(
						nil, nil,
						widget.NewLabel(fw.Label+":"), nil,
						fw.Widget,
					)
					items = append(items, formItem)
				}
			} else {
				// Nested message - create expandable section
				nestedWidget := NewNestedMessageWidget(fieldName, fd.Message())
				b.nestedFields[fieldName] = nestedWidget
				items = append(items, nestedWidget)
			}

		} else {
			// Scalar field - use mapper
			fw := MapFieldToWidget(fd)
			if fw != nil {
				b.fields[fieldName] = fw

				// Create form row with label and widget
				var formItem fyne.CanvasObject
				if fd.Kind() == protoreflect.BoolKind {
					// Checkbox already has label
					formItem = fw.Widget
				} else {
					// Add label for other widgets
					formItem = container.NewBorder(
						nil, nil,
						widget.NewLabel(fw.Label+":"), nil,
						fw.Widget,
					)
				}
				items = append(items, formItem)
			}
		}
	}

	// Handle oneofs
	oneofs := b.md.Oneofs()
	for i := 0; i < oneofs.Len(); i++ {
		od := oneofs.Get(i)
		oneofName := string(od.Name())

		oneofWidget := NewOneofWidget(oneofName, od)
		b.oneofFields[oneofName] = oneofWidget
		items = append(items, oneofWidget)
	}

	// If no fields, show placeholder
	if len(items) == 0 {
		items = append(items, widget.NewLabel("(empty message)"))
	}

	// Create scrollable container with all fields
	b.container = container.NewVBox(items...)
	return container.NewVScroll(b.container)
}

// GetFields returns all field widgets
func (b *FormBuilder) GetFields() []*FieldWidget {
	fields := make([]*FieldWidget, 0, len(b.fields))
	for _, fw := range b.fields {
		fields = append(fields, fw)
	}
	return fields
}

// GetValues collects all field values into a map
func (b *FormBuilder) GetValues() map[string]interface{} {
	values := make(map[string]interface{})

	// Collect scalar field values
	for name, fw := range b.fields {
		val := fw.GetValue()
		// Only include non-zero values
		if !isZeroValue(val) {
			values[name] = val
		}
	}

	// Collect repeated field values
	for name, rfw := range b.repeatedFields {
		val := rfw.GetValue()
		if items, ok := val.([]interface{}); ok && len(items) > 0 {
			values[name] = items
		}
	}

	// Collect nested message values
	for name, nfw := range b.nestedFields {
		val := nfw.GetValue()
		if nestedMap, ok := val.(map[string]interface{}); ok && len(nestedMap) > 0 {
			values[name] = nestedMap
		}
	}

	// Collect oneof values
	for _, ofw := range b.oneofFields {
		oneofVal := ofw.GetValue()
		if oneofVal != nil {
			if m, ok := oneofVal.(map[string]interface{}); ok {
				for k, v := range m {
					values[k] = v
				}
			}
		}
	}

	return values
}

// SetValues populates form fields from a map
func (b *FormBuilder) SetValues(values map[string]interface{}) {
	// Set scalar field values
	for name, fw := range b.fields {
		if val, ok := values[name]; ok {
			fw.SetValue(val)
		}
	}

	// Set repeated field values
	for name, rfw := range b.repeatedFields {
		if val, ok := values[name]; ok {
			rfw.SetValue(val)
		}
	}

	// Set nested message values
	for name, nfw := range b.nestedFields {
		if val, ok := values[name]; ok {
			nfw.SetValue(val)
		}
	}

	// Set oneof values
	for _, ofw := range b.oneofFields {
		// Check if any oneof field is present in values
		oneofDesc := ofw.GetDescriptor()
		fields := oneofDesc.Fields()
		for i := 0; i < fields.Len(); i++ {
			fd := fields.Get(i)
			fieldName := string(fd.Name())
			if val, ok := values[fieldName]; ok {
				ofw.SetValue(fieldName, val)
				break
			}
		}
	}
}

// ToJSON converts form values to JSON string
func (b *FormBuilder) ToJSON() (string, error) {
	// Create a dynamic message from the descriptor
	msg := dynamicpb.NewMessage(b.md)

	// Populate message from form values
	values := b.GetValues()
	if err := b.populateMessage(msg, values); err != nil {
		return "", fmt.Errorf("failed to populate message: %w", err)
	}

	// Marshal to JSON using protojson
	jsonBytes, err := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: false,
	}.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to JSON: %w", err)
	}

	return string(jsonBytes), nil
}

// FromJSON populates form from JSON string
func (b *FormBuilder) FromJSON(jsonStr string) error {
	// Create a dynamic message from the descriptor
	msg := dynamicpb.NewMessage(b.md)

	// Unmarshal JSON into message
	if err := protojson.Unmarshal([]byte(jsonStr), msg); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	// Extract values from message
	values := b.messageToMap(msg)

	// Populate form fields
	b.SetValues(values)

	return nil
}

// Clear resets all fields to defaults
func (b *FormBuilder) Clear() {
	// Clear scalar fields
	for _, fw := range b.fields {
		fw.SetValue(getDefaultValue(fw.Descriptor))
	}

	// Clear repeated fields
	for _, rfw := range b.repeatedFields {
		rfw.Clear()
	}

	// Clear nested messages
	for _, nfw := range b.nestedFields {
		if builder := nfw.GetBuilder(); builder != nil {
			builder.Clear()
		}
	}

	// Clear oneofs
	for _, ofw := range b.oneofFields {
		ofw.Clear()
	}
}

// populateMessage sets field values from a map to a proto message
func (b *FormBuilder) populateMessage(msg protoreflect.Message, values map[string]interface{}) error {
	for fieldName, value := range values {
		fd := b.md.Fields().ByName(protoreflect.Name(fieldName))
		if fd == nil {
			continue // Skip unknown fields
		}

		if err := setFieldValue(msg, fd, value); err != nil {
			return fmt.Errorf("failed to set field %s: %w", fieldName, err)
		}
	}
	return nil
}

// messageToMap converts a proto message to a map
func (b *FormBuilder) messageToMap(msg protoreflect.Message) map[string]interface{} {
	values := make(map[string]interface{})

	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		fieldName := string(fd.Name())
		values[fieldName] = valueToInterface(fd, v)
		return true
	})

	return values
}

// setFieldValue sets a field value in a proto message
func setFieldValue(msg protoreflect.Message, fd protoreflect.FieldDescriptor, value interface{}) error {
	if fd.IsList() {
		// Handle repeated fields
		list := msg.Mutable(fd).List()
		if slice, ok := value.([]interface{}); ok {
			for _, item := range slice {
				val, err := interfaceToValue(fd, item)
				if err != nil {
					return err
				}
				list.Append(val)
			}
		}
		return nil
	}

	if fd.IsMap() {
		// Handle map fields
		// TODO: Implement map support
		return nil
	}

	// Handle scalar and message fields
	val, err := interfaceToValue(fd, value)
	if err != nil {
		return err
	}
	msg.Set(fd, val)
	return nil
}

// interfaceToValue converts a Go interface{} to a protoreflect.Value
func interfaceToValue(fd protoreflect.FieldDescriptor, v interface{}) (protoreflect.Value, error) {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		if b, ok := v.(bool); ok {
			return protoreflect.ValueOfBool(b), nil
		}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		if i, ok := v.(int32); ok {
			return protoreflect.ValueOfInt32(i), nil
		}
		if i, ok := v.(float64); ok {
			return protoreflect.ValueOfInt32(int32(i)), nil
		}
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		if i, ok := v.(int64); ok {
			return protoreflect.ValueOfInt64(i), nil
		}
		if i, ok := v.(float64); ok {
			return protoreflect.ValueOfInt64(int64(i)), nil
		}
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		if u, ok := v.(uint32); ok {
			return protoreflect.ValueOfUint32(u), nil
		}
		if f, ok := v.(float64); ok {
			return protoreflect.ValueOfUint32(uint32(f)), nil
		}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		if u, ok := v.(uint64); ok {
			return protoreflect.ValueOfUint64(u), nil
		}
		if f, ok := v.(float64); ok {
			return protoreflect.ValueOfUint64(uint64(f)), nil
		}
	case protoreflect.FloatKind:
		if f, ok := v.(float32); ok {
			return protoreflect.ValueOfFloat32(f), nil
		}
		if f, ok := v.(float64); ok {
			return protoreflect.ValueOfFloat32(float32(f)), nil
		}
	case protoreflect.DoubleKind:
		if f, ok := v.(float64); ok {
			return protoreflect.ValueOfFloat64(f), nil
		}
	case protoreflect.StringKind:
		if s, ok := v.(string); ok {
			return protoreflect.ValueOfString(s), nil
		}
	case protoreflect.BytesKind:
		if b, ok := v.([]byte); ok {
			return protoreflect.ValueOfBytes(b), nil
		}
	case protoreflect.EnumKind:
		if i, ok := v.(int32); ok {
			return protoreflect.ValueOfEnum(protoreflect.EnumNumber(i)), nil
		}
		if f, ok := v.(float64); ok {
			return protoreflect.ValueOfEnum(protoreflect.EnumNumber(int32(f))), nil
		}
	case protoreflect.MessageKind:
		// Handle nested messages
		if m, ok := v.(map[string]interface{}); ok {
			nestedMsg := dynamicpb.NewMessage(fd.Message())
			for k, val := range m {
				nestedFd := fd.Message().Fields().ByName(protoreflect.Name(k))
				if nestedFd != nil {
					if err := setFieldValue(nestedMsg, nestedFd, val); err != nil {
						return protoreflect.Value{}, err
					}
				}
			}
			return protoreflect.ValueOfMessage(nestedMsg), nil
		}
	}

	return protoreflect.Value{}, fmt.Errorf("unsupported type conversion for %v", v)
}

// valueToInterface converts a protoreflect.Value to a Go interface{}
func valueToInterface(fd protoreflect.FieldDescriptor, v protoreflect.Value) interface{} {
	if fd.IsList() {
		list := v.List()
		result := make([]interface{}, list.Len())
		for i := 0; i < list.Len(); i++ {
			result[i] = valueToInterface(fd, list.Get(i))
		}
		return result
	}

	switch fd.Kind() {
	case protoreflect.BoolKind:
		return v.Bool()
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return int32(v.Int())
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return v.Int()
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return uint32(v.Uint())
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return v.Uint()
	case protoreflect.FloatKind:
		return float32(v.Float())
	case protoreflect.DoubleKind:
		return v.Float()
	case protoreflect.StringKind:
		return v.String()
	case protoreflect.BytesKind:
		return v.Bytes()
	case protoreflect.EnumKind:
		return int32(v.Enum())
	case protoreflect.MessageKind:
		msg := v.Message()
		result := make(map[string]interface{})
		msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			result[string(fd.Name())] = valueToInterface(fd, v)
			return true
		})
		return result
	}

	return nil
}

// isZeroValue checks if a value is the zero value for its type
func isZeroValue(v interface{}) bool {
	if v == nil {
		return true
	}

	switch val := v.(type) {
	case bool:
		return !val
	case int32, int64, uint32, uint64:
		return val == 0
	case float32, float64:
		return val == 0
	case string:
		return val == ""
	case []byte:
		return len(val) == 0
	}

	return false
}

// getDefaultValue returns the default value for a field descriptor
func getDefaultValue(fd protoreflect.FieldDescriptor) interface{} {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return false
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return int32(0)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return int64(0)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return uint32(0)
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return uint64(0)
	case protoreflect.FloatKind:
		return float32(0)
	case protoreflect.DoubleKind:
		return float64(0)
	case protoreflect.StringKind:
		return ""
	case protoreflect.BytesKind:
		return []byte{}
	case protoreflect.EnumKind:
		// Return first enum value
		return int32(0)
	}
	return nil
}

// Validate validates all form fields
func (b *FormBuilder) Validate() error {
	// Validate scalar fields
	for fieldName, fw := range b.fields {
		if fw.Validate != nil {
			if err := fw.Validate(); err != nil {
				return fmt.Errorf("field %s: %w", fieldName, err)
			}
		}
	}

	// Validate nested messages
	for fieldName, nfw := range b.nestedFields {
		if builder := nfw.GetBuilder(); builder != nil {
			if err := builder.Validate(); err != nil {
				return fmt.Errorf("nested field %s: %w", fieldName, err)
			}
		}
	}

	return nil
}

// ToMap converts form values to a generic map (useful for JSON serialization)
func (b *FormBuilder) ToMap() (map[string]interface{}, error) {
	values := b.GetValues()
	return values, nil
}

// FromMap populates form from a generic map
func (b *FormBuilder) FromMap(values map[string]interface{}) error {
	b.SetValues(values)
	return nil
}

// BuildForm creates a form for a message descriptor (alias for Build for API compatibility)
func (b *FormBuilder) BuildForm(md protoreflect.MessageDescriptor) fyne.CanvasObject {
	// If a different descriptor is provided, recreate the builder
	if md != b.md {
		newBuilder := NewFormBuilder(md)
		*b = *newBuilder
	}
	return b.Build()
}
