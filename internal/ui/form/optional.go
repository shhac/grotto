package form

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/shhac/grotto/internal/ui/components"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// OptionalFieldWidget wraps a field with a checkbox toggle for field presence.
// Used for proto3 optional fields and single-member oneofs.
// When unchecked, the field is omitted from the request entirely.
// When checked, the field is included even if set to its zero value.
type OptionalFieldWidget struct {
	widget.BaseWidget

	name    string
	toggle  *widget.Check
	content *fyne.Container // inner content, shown/hidden with toggle
	outer   *fyne.Container // full layout

	getInnerValue func() interface{}
	setInnerValue func(interface{})
	clearInner    func()
}

// NewOptionalScalarWidget creates an optional toggle wrapping a scalar FieldWidget.
// The toggle controls field presence; the inner widget provides the value.
func NewOptionalScalarWidget(fw *FieldWidget) *OptionalFieldWidget {
	o := &OptionalFieldWidget{name: fw.Name}

	// For bool fields, strip the inner check label since the toggle provides it
	if check, ok := fw.Widget.(*widget.Check); ok {
		check.Text = ""
		check.Refresh()
	}

	o.toggle = widget.NewCheck(fw.Label, nil)
	typeHint := components.NewHintLabel(scalarTypeHint(fw.Descriptor))
	toggleRow := container.NewHBox(o.toggle, typeHint)

	o.content = container.NewStack(fw.Widget)
	o.content.Hide()

	o.outer = container.NewBorder(nil, nil, toggleRow, nil, o.content)

	// Set callback after outer is created so refresh works
	o.toggle.OnChanged = func(checked bool) {
		if checked {
			o.content.Show()
		} else {
			o.content.Hide()
		}
		o.outer.Refresh()
	}

	o.getInnerValue = fw.GetValue
	o.setInnerValue = fw.SetValue
	o.clearInner = func() { fw.SetValue(getDefaultValue(fw.Descriptor)) }

	o.ExtendBaseWidget(o)
	return o
}

// NewOptionalNestedWidget creates an optional toggle wrapping a nested message.
// When toggled on, all sub-fields of the message are shown indented below the toggle.
func NewOptionalNestedWidget(name string, md protoreflect.MessageDescriptor) *OptionalFieldWidget {
	o := &OptionalFieldWidget{name: name}

	builder := NewFormBuilder(md)

	o.toggle = widget.NewCheck(formatFieldLabel(name), nil)
	typeHint := components.NewHintLabel(string(md.Name()))

	// Indent nested content for visual depth cue
	leftPad := canvas.NewRectangle(color.Transparent)
	leftPad.SetMinSize(fyne.NewSize(12, 0))
	indented := container.NewBorder(nil, nil, leftPad, nil, builder.BuildContent())

	o.content = container.NewVBox(indented)
	o.content.Hide()

	o.outer = container.NewVBox(container.NewHBox(o.toggle, typeHint), o.content)

	o.toggle.OnChanged = func(checked bool) {
		if checked {
			o.content.Show()
		} else {
			o.content.Hide()
		}
		o.outer.Refresh()
	}

	o.getInnerValue = func() interface{} { return builder.GetValues() }
	o.setInnerValue = func(v interface{}) {
		if m, ok := v.(map[string]interface{}); ok {
			builder.SetValues(m)
		}
	}
	o.clearInner = func() { builder.Clear() }

	o.ExtendBaseWidget(o)
	return o
}

// IsEnabled returns whether the optional field toggle is checked.
func (o *OptionalFieldWidget) IsEnabled() bool {
	return o.toggle.Checked
}

// SetEnabled toggles the optional field on or off.
func (o *OptionalFieldWidget) SetEnabled(enabled bool) {
	o.toggle.SetChecked(enabled)
}

// GetValue returns the inner value if enabled, or nil if disabled.
func (o *OptionalFieldWidget) GetValue() interface{} {
	if !o.toggle.Checked {
		return nil
	}
	return o.getInnerValue()
}

// SetValue enables the toggle and sets the inner value.
func (o *OptionalFieldWidget) SetValue(v interface{}) {
	o.toggle.SetChecked(true)
	o.setInnerValue(v)
}

// Clear disables the toggle and resets the inner value.
func (o *OptionalFieldWidget) Clear() {
	o.toggle.SetChecked(false)
	o.clearInner()
}

// CreateRenderer implements fyne.Widget.
func (o *OptionalFieldWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(o.outer)
}
