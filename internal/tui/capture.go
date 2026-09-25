// Package tui is the capture screen. It owns no files: it collects text and
// reports what the user decided, which is what makes it testable without a
// terminal.
package tui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// chrome is the header and footer lines plus the blank line under the header.
const chrome = 4

// linkOpener is what, typed before the cursor, opens the link picker.
const linkOpener = "[["

var headerStyle = lipgloss.NewStyle().Faint(true)

// Result is what the user decided.
type Result struct {
	Body  string
	Saved bool
}

// Model is the capture screen.
type Model struct {
	textarea textarea.Model
	help     help.Model
	keys     keyMap
	header   string
	width    int
	saved    bool

	links   LinkSource
	picker  picker
	picking bool

	// opened records that the picker was opened by typing linkOpener, which
	// the chosen link then replaces.
	opened bool
}

// New builds the capture screen. The header shows where the note will land, so
// the answer to "where did that go" is on screen while typing.
//
// links lists the notes that can be linked to. With nil there is no picker, and
// brackets are only text.
func New(zettelID, noteType, path string, links LinkSource) Model {
	ta := textarea.New()
	ta.Placeholder = "what are you thinking?"
	ta.ShowLineNumbers = false
	ta.Focus()

	// The defaults cap at 99 rows and 500 columns, which silently stops a long
	// note from growing.
	ta.CharLimit = 0

	k := keys
	k.Link.SetEnabled(links != nil)

	return Model{
		textarea: ta,
		help:     help.New(),
		keys:     k,
		header:   strings.Join([]string{zettelID, noteType, path}, " · "),
		links:    links,
		picker:   newPicker(),
	}
}

// The background color decides the textarea's palette: lipgloss no longer
// detects it, so we ask for it and restyle when the answer arrives.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink, tea.RequestBackgroundColor}
	if m.links != nil {
		cmds = append(cmds, m.links.load())
	}

	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.textarea.SetStyles(textarea.DefaultStyles(msg.IsDark()))
		m.picker.setStyles(msg.IsDark())
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.help.SetWidth(msg.Width)

		// The setters clamp against the maxima, so the maxima go first.
		m.textarea.MaxWidth = msg.Width
		m.textarea.SetWidth(msg.Width)

		height := max(msg.Height-chrome, 1)
		m.textarea.MaxHeight = height
		m.textarea.SetHeight(height)
		m.picker.setSize(msg.Width, height)

		return m, nil

	case linksLoadedMsg:
		m.picker.setLinks(msg.links, msg.err)
		return m, nil

	case tea.KeyPressMsg:
		if m.picking {
			return m.updatePicker(msg)
		}

		switch {
		case key.Matches(msg, m.keys.Save):
			m.saved = true
			return m, tea.Quit

		case key.Matches(msg, m.keys.Discard):
			m.saved = false
			return m, tea.Quit

		case key.Matches(msg, m.keys.Link):
			return m.openPicker(false)
		}
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)

	// Only typing the opener's last character counts. A paste arrives whole,
	// and moving the cursor past brackets already written is not a request to
	// link.
	if press, ok := msg.(tea.KeyPressMsg); ok && m.links != nil &&
		press.Text == linkOpener[len(linkOpener)-1:] && m.openerBeforeCursor() {
		m, open := m.openPicker(true)
		return m, tea.Batch(cmd, open)
	}

	return m, cmd
}

func (m Model) updatePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, pickerKeys.Accept):
		link, ok := m.picker.selected()
		opened := m.opened
		m.closePicker()

		if !ok {
			return m, nil
		}

		if opened {
			for range linkOpener {
				m.textarea, _ = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
			}
		}

		m.textarea.InsertString(link.Text)

		return m, nil

	case key.Matches(msg, pickerKeys.Cancel):
		m.closePicker()
		return m, nil

	case key.Matches(msg, pickerKeys.Next):
		m.picker.move(1)
		return m, nil

	case key.Matches(msg, pickerKeys.Prev):
		m.picker.move(-1)
		return m, nil

	// Saving from inside the picker would lose the half-typed query's
	// intent, so it waits until the picker closes.
	case key.Matches(msg, m.keys.Save):
		return m, nil
	}

	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)

	return m, cmd
}

func (m Model) openPicker(opened bool) (Model, tea.Cmd) {
	m.picking, m.opened = true, opened
	return m, m.picker.open()
}

func (m *Model) closePicker() {
	m.picking, m.opened = false, false
	m.picker.close()
}

// openerBeforeCursor reports whether the text just before the cursor, on the
// cursor's line, is linkOpener.
func (m Model) openerBeforeCursor() bool {
	lines := strings.Split(m.textarea.Value(), "\n")
	if m.textarea.Line() >= len(lines) {
		return false
	}

	line := []rune(lines[m.textarea.Line()])
	col := min(m.textarea.Column(), len(line))

	return strings.HasSuffix(string(line[:col]), linkOpener)
}

func (m Model) View() tea.View {
	body, keys := m.textarea.View(), help.KeyMap(m.keys)
	if m.picking {
		body, keys = m.picker.View(), pickerKeys
	}

	v := tea.NewView(strings.Join([]string{
		headerStyle.Render(m.header),
		"",
		body,
		m.help.View(keys),
	}, "\n"))

	v.AltScreen = true

	return v
}

// Result reports what the user decided.
func (m Model) Result() Result {
	return Result{Body: m.textarea.Value(), Saved: m.saved}
}
