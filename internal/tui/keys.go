package tui

import "charm.land/bubbles/v2/key"

// keyMap is the capture screen's bindings. Enter is deliberately absent: this
// is a multi-line editor, and saving on enter would fight the thing it is for.
type keyMap struct {
	Save    key.Binding
	Discard key.Binding

	// Link is disabled when there is nothing to link to, which hides it from
	// help as well as from matching.
	Link key.Binding
}

var keys = keyMap{
	Save: key.NewBinding(
		// ctrl+d as well, for the EOF reflex.
		key.WithKeys("ctrl+s", "ctrl+d"),
		key.WithHelp("ctrl+s", "save"),
	),
	Discard: key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c", "discard"),
	),
	Link: key.NewBinding(
		key.WithKeys("ctrl+l"),
		key.WithHelp("ctrl+l", "link"),
	),
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Save, k.Discard, k.Link}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

// pickerKeyMap is the link picker's bindings. Cancel shares its keys with
// Discard on purpose: while the picker is open they close the picker, never
// the note.
type pickerKeyMap struct {
	Accept key.Binding
	Cancel key.Binding
	Next   key.Binding
	Prev   key.Binding
}

var pickerKeys = pickerKeyMap{
	Accept: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "insert"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("esc", "cancel"),
	),
	Next: key.NewBinding(
		key.WithKeys("down", "ctrl+n"),
		key.WithHelp("↓", "next"),
	),
	Prev: key.NewBinding(
		key.WithKeys("up", "ctrl+p"),
		key.WithHelp("↑", "prev"),
	),
}

func (k pickerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Accept, k.Cancel, k.Next, k.Prev}
}

func (k pickerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}
