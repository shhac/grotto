package form

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// FieldWidget represents a form field with its widget and value binding
type FieldWidget struct {
	Name       string
	Label      string
	Widget     fyne.CanvasObject
	Descriptor protoreflect.FieldDescriptor

	// Value getters/setters
	GetValue func() interface{}
	SetValue func(interface{})
	Validate func() error
}

// MapFieldToWidget creates a Fyne widget for a proto field
// Returns nil for repeated fields, maps, and nested messages (handled by builder)
func MapFieldToWidget(fd protoreflect.FieldDescriptor) *FieldWidget {
	// Skip repeated fields and maps - these need special container handling in builder
	if fd.IsList() || fd.IsMap() {
		return nil
	}

	// Skip nested messages - these need recursive form generation in builder
	if fd.Kind() == protoreflect.MessageKind && !isWellKnownType(fd) {
		return nil
	}

	name := string(fd.Name())
	label := formatFieldLabel(name)

	fw := &FieldWidget{
		Name:       name,
		Label:      label,
		Descriptor: fd,
	}

	// Map proto type to widget
	switch fd.Kind() {
	case protoreflect.BoolKind:
		check := widget.NewCheck(label, nil)
		fw.Widget = check
		fw.GetValue = func() interface{} { return check.Checked }
		fw.SetValue = func(v interface{}) {
			if b, ok := v.(bool); ok {
				check.SetChecked(b)
			}
		}
		fw.Validate = func() error { return nil }

	case protoreflect.EnumKind:
		// Build enum options
		enumDesc := fd.Enum()
		values := enumDesc.Values()
		options := make([]string, values.Len())
		for i := 0; i < values.Len(); i++ {
			val := values.Get(i)
			options[i] = string(val.Name())
		}

		sel := widget.NewSelect(options, nil)
		if len(options) > 0 {
			sel.SetSelected(options[0]) // Default to first enum value
		}

		fw.Widget = sel
		fw.GetValue = func() interface{} {
			// Return enum number
			for i := 0; i < values.Len(); i++ {
				val := values.Get(i)
				if string(val.Name()) == sel.Selected {
					return int32(val.Number())
				}
			}
			return int32(0)
		}
		fw.SetValue = func(v interface{}) {
			var enumNum int32
			switch t := v.(type) {
			case int32:
				enumNum = t
			case int:
				enumNum = int32(t)
			default:
				return
			}

			// Find enum name by number
			for i := 0; i < values.Len(); i++ {
				val := values.Get(i)
				if val.Number() == protoreflect.EnumNumber(enumNum) {
					sel.SetSelected(string(val.Name()))
					return
				}
			}
		}
		fw.Validate = func() error { return nil }

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		entry := widget.NewEntry()
		entry.SetPlaceHolder("0")
		fw.Widget = entry
		fw.GetValue = func() interface{} {
			if entry.Text == "" {
				return int32(0)
			}
			val, _ := strconv.ParseInt(entry.Text, 10, 32)
			return int32(val)
		}
		fw.SetValue = func(v interface{}) {
			if num, ok := v.(int32); ok {
				entry.SetText(strconv.FormatInt(int64(num), 10))
			}
		}
		fw.Validate = func() error {
			if entry.Text == "" {
				return nil // Optional field
			}
			return ValidateInt32(entry.Text)
		}

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		entry := widget.NewEntry()
		entry.SetPlaceHolder("0")
		fw.Widget = entry
		fw.GetValue = func() interface{} {
			if entry.Text == "" {
				return int64(0)
			}
			val, _ := strconv.ParseInt(entry.Text, 10, 64)
			return val
		}
		fw.SetValue = func(v interface{}) {
			if num, ok := v.(int64); ok {
				entry.SetText(strconv.FormatInt(num, 10))
			}
		}
		fw.Validate = func() error {
			if entry.Text == "" {
				return nil
			}
			return ValidateInt64(entry.Text)
		}

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		entry := widget.NewEntry()
		entry.SetPlaceHolder("0")
		fw.Widget = entry
		fw.GetValue = func() interface{} {
			if entry.Text == "" {
				return uint32(0)
			}
			val, _ := strconv.ParseUint(entry.Text, 10, 32)
			return uint32(val)
		}
		fw.SetValue = func(v interface{}) {
			if num, ok := v.(uint32); ok {
				entry.SetText(strconv.FormatUint(uint64(num), 10))
			}
		}
		fw.Validate = func() error {
			if entry.Text == "" {
				return nil
			}
			return ValidateUint32(entry.Text)
		}

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		entry := widget.NewEntry()
		entry.SetPlaceHolder("0")
		fw.Widget = entry
		fw.GetValue = func() interface{} {
			if entry.Text == "" {
				return uint64(0)
			}
			val, _ := strconv.ParseUint(entry.Text, 10, 64)
			return val
		}
		fw.SetValue = func(v interface{}) {
			if num, ok := v.(uint64); ok {
				entry.SetText(strconv.FormatUint(num, 10))
			}
		}
		fw.Validate = func() error {
			if entry.Text == "" {
				return nil
			}
			return ValidateUint64(entry.Text)
		}

	case protoreflect.FloatKind:
		entry := widget.NewEntry()
		entry.SetPlaceHolder("0.0")
		fw.Widget = entry
		fw.GetValue = func() interface{} {
			if entry.Text == "" {
				return float32(0)
			}
			val, _ := strconv.ParseFloat(entry.Text, 32)
			return float32(val)
		}
		fw.SetValue = func(v interface{}) {
			if num, ok := v.(float32); ok {
				entry.SetText(strconv.FormatFloat(float64(num), 'g', -1, 32))
			}
		}
		fw.Validate = func() error {
			if entry.Text == "" {
				return nil
			}
			return ValidateFloat(entry.Text)
		}

	case protoreflect.DoubleKind:
		entry := widget.NewEntry()
		entry.SetPlaceHolder("0.0")
		fw.Widget = entry
		fw.GetValue = func() interface{} {
			if entry.Text == "" {
				return float64(0)
			}
			val, _ := strconv.ParseFloat(entry.Text, 64)
			return val
		}
		fw.SetValue = func(v interface{}) {
			if num, ok := v.(float64); ok {
				entry.SetText(strconv.FormatFloat(num, 'g', -1, 64))
			}
		}
		fw.Validate = func() error {
			if entry.Text == "" {
				return nil
			}
			return ValidateDouble(entry.Text)
		}

	case protoreflect.StringKind:
		entry := widget.NewEntry()
		entry.SetPlaceHolder("Enter text")
		fw.Widget = entry
		fw.GetValue = func() interface{} { return entry.Text }
		fw.SetValue = func(v interface{}) {
			if s, ok := v.(string); ok {
				entry.SetText(s)
			}
		}
		fw.Validate = func() error { return nil }

	case protoreflect.BytesKind:
		// Entry with base64 encoding hint
		entry := widget.NewEntry()
		entry.SetPlaceHolder("Base64 encoded bytes")
		fw.Widget = entry
		fw.GetValue = func() interface{} {
			if entry.Text == "" {
				return []byte{}
			}
			// Try to decode base64
			data, err := base64.StdEncoding.DecodeString(entry.Text)
			if err != nil {
				// If not valid base64, treat as raw string
				return []byte(entry.Text)
			}
			return data
		}
		fw.SetValue = func(v interface{}) {
			if b, ok := v.([]byte); ok {
				entry.SetText(base64.StdEncoding.EncodeToString(b))
			}
		}
		fw.Validate = func() error {
			if entry.Text == "" {
				return nil
			}
			_, err := base64.StdEncoding.DecodeString(entry.Text)
			return err
		}

	case protoreflect.MessageKind:
		// Handle well-known types
		msgType := fd.Message().FullName()
		switch msgType {
		case "google.protobuf.Timestamp":
			entry := widget.NewEntry()
			entry.SetPlaceHolder("RFC3339 format (e.g., 2024-01-15T10:30:00Z)")
			fw.Widget = entry
			fw.GetValue = func() interface{} { return entry.Text }
			fw.SetValue = func(v interface{}) {
				if s, ok := v.(string); ok {
					entry.SetText(s)
				}
			}
			fw.Validate = func() error { return nil } // TODO: Add RFC3339 validation

		case "google.protobuf.Duration":
			entry := widget.NewEntry()
			entry.SetPlaceHolder("Duration format (e.g., 5m30s)")
			fw.Widget = entry
			fw.GetValue = func() interface{} { return entry.Text }
			fw.SetValue = func(v interface{}) {
				if s, ok := v.(string); ok {
					entry.SetText(s)
				}
			}
			fw.Validate = func() error { return nil } // TODO: Add duration validation

		case "google.protobuf.FieldMask":
			entry := widget.NewMultiLineEntry()
			entry.SetPlaceHolder("Field paths (one per line or comma-separated)")
			fw.Widget = entry
			fw.GetValue = func() interface{} {
				text := strings.TrimSpace(entry.Text)
				if text == "" {
					return []string{}
				}
				// Support both newline and comma-separated formats
				var paths []string
				if strings.Contains(text, "\n") {
					// Multi-line format: one path per line
					lines := strings.Split(text, "\n")
					for _, line := range lines {
						line = strings.TrimSpace(line)
						if line != "" {
							paths = append(paths, line)
						}
					}
				} else {
					// Comma-separated format
					parts := strings.Split(text, ",")
					for _, part := range parts {
						part = strings.TrimSpace(part)
						if part != "" {
							paths = append(paths, part)
						}
					}
				}
				return paths
			}
			fw.SetValue = func(v interface{}) {
				// Accept either []string or string
				switch val := v.(type) {
				case []string:
					if len(val) == 0 {
						entry.SetText("")
					} else {
						// Format as one path per line
						entry.SetText(strings.Join(val, "\n"))
					}
				case string:
					entry.SetText(val)
				}
			}
			fw.Validate = func() error { return nil }

		default:
			// Unknown message type - should be handled by builder
			return nil
		}

	default:
		// Unsupported type - shouldn't happen with proto reflection
		entry := widget.NewEntry()
		entry.SetPlaceHolder(fmt.Sprintf("Unsupported type: %s", fd.Kind()))
		entry.Disable()
		fw.Widget = entry
		fw.GetValue = func() interface{} { return nil }
		fw.SetValue = func(v interface{}) {}
		fw.Validate = func() error { return nil }
	}

	return fw
}

// formatFieldLabel converts snake_case field names to Title Case labels
func formatFieldLabel(fieldName string) string {
	// Split on underscores
	parts := strings.Split(fieldName, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

// isWellKnownType checks if a message field is a well-known type
func isWellKnownType(fd protoreflect.FieldDescriptor) bool {
	if fd.Kind() != protoreflect.MessageKind {
		return false
	}
	fullName := fd.Message().FullName()
	return strings.HasPrefix(string(fullName), "google.protobuf.")
}
