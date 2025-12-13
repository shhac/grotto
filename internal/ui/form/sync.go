package form

import (
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// FormSync manages bidirectional sync between JSON text and form widgets
type FormSync struct {
	textBinding binding.String // JSON text from text mode
	builder     *FormBuilder   // Form builder
	md          protoreflect.MessageDescriptor

	// Prevents infinite sync loops
	syncing bool

	// Listeners for cleanup
	textListener    binding.DataListener
	fieldListeners  []binding.DataListener
	changeCallbacks []func() // Callbacks for field changes
}

// NewFormSync creates a new form sync manager
func NewFormSync(textBinding binding.String, md protoreflect.MessageDescriptor) *FormSync {
	s := &FormSync{
		textBinding:     textBinding,
		md:              md,
		changeCallbacks: make([]func(), 0),
	}

	// Create form builder
	s.builder = NewFormBuilder(md)

	return s
}

// SetMessageDescriptor updates for new method selection
func (s *FormSync) SetMessageDescriptor(md protoreflect.MessageDescriptor) {
	// Disable auto-sync during rebuild
	s.DisableAutoSync()

	// Update descriptor and rebuild form
	s.md = md
	s.builder = NewFormBuilder(md)

	// Clear text binding
	s.textBinding.Set("{}")

	// Re-enable auto-sync with new form
	s.SetupAutoSync()
}

// GetFormBuilder returns the form builder (create form UI from this)
func (s *FormSync) GetFormBuilder() *FormBuilder {
	return s.builder
}

// SyncTextToForm updates form from current JSON text
func (s *FormSync) SyncTextToForm() error {
	// Prevent infinite loops
	if s.syncing {
		return nil
	}
	s.syncing = true
	defer func() { s.syncing = false }()

	// Get JSON from text binding
	jsonText, err := s.textBinding.Get()
	if err != nil {
		return err
	}

	// Handle empty text
	if jsonText == "" || jsonText == "{}" {
		s.builder.Clear()
		return nil
	}

	// Parse JSON and populate form
	if err := s.builder.FromJSON(jsonText); err != nil {
		// Don't update form on invalid JSON - let text mode show the error
		return err
	}

	return nil
}

// SyncFormToText updates JSON text from current form values
func (s *FormSync) SyncFormToText() error {
	// Prevent infinite loops
	if s.syncing {
		return nil
	}
	s.syncing = true
	defer func() { s.syncing = false }()

	// Convert form to JSON
	jsonText, err := s.builder.ToJSON()
	if err != nil {
		// Still try to update text even on error - helps with debugging
		return err
	}

	// Update text binding
	return s.textBinding.Set(jsonText)
}

// SetupAutoSync sets up listeners for automatic sync
// When text changes -> update form
// When form field changes -> update text
func (s *FormSync) SetupAutoSync() {
	// Add listener for text changes
	s.textListener = binding.NewDataListener(func() {
		// Sync text to form when text changes
		s.SyncTextToForm()
	})
	s.textBinding.AddListener(s.textListener)

	// Add change callbacks for form fields
	// When any field changes, we sync form to text
	s.setupFieldChangeCallbacks()
}

// setupFieldChangeCallbacks sets up change callbacks for all form fields
func (s *FormSync) setupFieldChangeCallbacks() {
	// Create a single callback that syncs form to text
	syncCallback := func() {
		s.SyncFormToText()
	}

	// Add callback to all scalar fields by wrapping their widgets
	// We attach OnChanged handlers to the underlying Fyne widgets
	s.attachWidgetCallbacks(syncCallback)

	// Add callback to all repeated fields
	for _, rfw := range s.builder.repeatedFields {
		// Use the setter methods instead of direct assignment
		rfw.OnAdd(syncCallback)
		rfw.OnRemove(func(int) { syncCallback() })
	}

	// Add callback to all nested message fields
	for _, nfw := range s.builder.nestedFields {
		if builder := nfw.GetBuilder(); builder != nil {
			// Recursively setup callbacks for nested fields
			s.setupNestedFieldCallbacks(builder, syncCallback)
		}
	}

	// For oneof fields, we need to listen to the selector change
	for _, ofw := range s.builder.oneofFields {
		// Get the selector widget and add a change callback
		// Since we can't directly modify SetValue, we'll wrap the selector's OnChanged
		s.wrapOneofSelector(ofw, syncCallback)
	}
}

// attachWidgetCallbacks attaches onChange callbacks to the underlying Fyne widgets
func (s *FormSync) attachWidgetCallbacks(syncCallback func()) {
	for _, fw := range s.builder.fields {
		// Attach callbacks based on widget type
		switch w := fw.Widget.(type) {
		case *widget.Entry:
			// Wrap existing OnChanged if present
			existingOnChanged := w.OnChanged
			w.OnChanged = func(text string) {
				if existingOnChanged != nil {
					existingOnChanged(text)
				}
				if !s.syncing {
					syncCallback()
				}
			}
		case *widget.Check:
			existingOnChanged := w.OnChanged
			w.OnChanged = func(checked bool) {
				if existingOnChanged != nil {
					existingOnChanged(checked)
				}
				if !s.syncing {
					syncCallback()
				}
			}
		case *widget.Select:
			existingOnChanged := w.OnChanged
			w.OnChanged = func(selected string) {
				if existingOnChanged != nil {
					existingOnChanged(selected)
				}
				if !s.syncing {
					syncCallback()
				}
			}
		}
	}
}

// wrapOneofSelector wraps the oneof selector's OnChanged callback
func (s *FormSync) wrapOneofSelector(ofw *OneofWidget, syncCallback func()) {
	// Access the selector field via reflection would be unsafe
	// Instead, we rely on the fact that oneof changes trigger field widget changes
	// which are already monitored by attachWidgetCallbacks
	// So we don't need to do anything special here
}

// setupNestedFieldCallbacks recursively sets up callbacks for nested message fields
func (s *FormSync) setupNestedFieldCallbacks(builder *FormBuilder, syncCallback func()) {
	// Attach callbacks to scalar field widgets
	for _, fw := range builder.fields {
		switch w := fw.Widget.(type) {
		case *widget.Entry:
			existingOnChanged := w.OnChanged
			w.OnChanged = func(text string) {
				if existingOnChanged != nil {
					existingOnChanged(text)
				}
				if !s.syncing {
					syncCallback()
				}
			}
		case *widget.Check:
			existingOnChanged := w.OnChanged
			w.OnChanged = func(checked bool) {
				if existingOnChanged != nil {
					existingOnChanged(checked)
				}
				if !s.syncing {
					syncCallback()
				}
			}
		case *widget.Select:
			existingOnChanged := w.OnChanged
			w.OnChanged = func(selected string) {
				if existingOnChanged != nil {
					existingOnChanged(selected)
				}
				if !s.syncing {
					syncCallback()
				}
			}
		}
	}

	// Add callback to all repeated fields
	for _, rfw := range builder.repeatedFields {
		rfw.OnAdd(syncCallback)
		rfw.OnRemove(func(int) { syncCallback() })
	}

	// Recurse into nested messages
	for _, nfw := range builder.nestedFields {
		if nestedBuilder := nfw.GetBuilder(); nestedBuilder != nil {
			s.setupNestedFieldCallbacks(nestedBuilder, syncCallback)
		}
	}
}

// DisableAutoSync removes auto-sync listeners (for manual control)
func (s *FormSync) DisableAutoSync() {
	// Remove text listener
	if s.textListener != nil {
		s.textBinding.RemoveListener(s.textListener)
		s.textListener = nil
	}

	// Clear change callbacks
	s.changeCallbacks = make([]func(), 0)

	// Note: We don't unwrap the SetValue functions here because they
	// check the syncing flag, so they won't cause issues
}
