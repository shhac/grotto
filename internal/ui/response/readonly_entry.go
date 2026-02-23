package response

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// ReadOnlyEntry is an Entry widget that appears fully enabled (normal contrast,
// native cursor) but prevents all text modifications. Text selection and copying
// still work normally.
type ReadOnlyEntry struct {
	widget.Entry
}

// NewReadOnlyMultiLineEntry creates a new multi-line read-only entry.
func NewReadOnlyMultiLineEntry() *ReadOnlyEntry {
	e := &ReadOnlyEntry{}
	e.MultiLine = true
	e.Wrapping = fyne.TextWrapWord
	e.ExtendBaseWidget(e)
	return e
}

// TypedRune blocks all character input.
func (e *ReadOnlyEntry) TypedRune(_ rune) {}

// TypedKey allows cursor/selection movement but blocks editing keys.
func (e *ReadOnlyEntry) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyLeft, fyne.KeyRight, fyne.KeyUp, fyne.KeyDown,
		fyne.KeyHome, fyne.KeyEnd, fyne.KeyPageUp, fyne.KeyPageDown:
		e.Entry.TypedKey(key)
	}
}

// TypedShortcut allows copy and select-all but blocks paste, cut, undo, redo.
func (e *ReadOnlyEntry) TypedShortcut(shortcut fyne.Shortcut) {
	switch shortcut.(type) {
	case *fyne.ShortcutCopy, *fyne.ShortcutSelectAll:
		e.Entry.TypedShortcut(shortcut)
	}
}
