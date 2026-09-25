// Package tui is the capture screen. It owns no files: it collects text and
// reports what the user decided, which is what makes it testable without a
// terminal.
package tui

import (
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// chrome is the header and footer lines plus the blank line under the header.
const chrome = 4

// bodyTop is the screen line the textarea starts on, under the header and the
// blank line.
const bodyTop = 2

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
	height   int
	saved    bool

	links   LinkSource
	picker  picker
	picking bool

	// anchorRow and anchorCol are where the query starts: everything typed
	// between there and the cursor is what the picker searches for.
	anchorRow, anchorCol int

	// opened records that the picker was opened by typing linkOpener, which
	// the chosen link then replaces along with the query.
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
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)

		// The setters clamp against the maxima, so the maxima go first.
		m.textarea.MaxWidth = msg.Width
		m.textarea.SetWidth(msg.Width)

		height := max(msg.Height-chrome, 1)
		m.textarea.MaxHeight = height
		m.textarea.SetHeight(height)

		return m, nil

	case linksLoadedMsg:
		m.picker.setLinks(msg.links, msg.err)
		return m, nil

	case tea.KeyPressMsg:
		if m.picking {
			var done bool
			if done, m = m.updatePicker(msg); done {
				return m, nil
			}
		}

		switch {
		case key.Matches(msg, m.keys.Save):
			m.saved = true
			return m, tea.Quit

		case key.Matches(msg, m.keys.Discard):
			m.saved = false
			return m, tea.Quit

		case !m.picking && key.Matches(msg, m.keys.Link):
			m.openPicker(false)
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)

	switch press, ok := msg.(tea.KeyPressMsg); {
	case m.picking:
		m.followQuery()

	// Only typing the opener's last character counts. A paste arrives whole,
	// and moving the cursor past brackets already written is not a request to
	// link.
	case ok && m.links != nil &&
		press.Text == linkOpener[len(linkOpener)-1:] && m.openerBeforeCursor():
		m.openPicker(true)
	}

	return m, cmd
}

// updatePicker handles the keys the picker claims while it is open. Anything
// it does not claim reports done as false and is typed into the note, which is
// how the query grows.
func (m Model) updatePicker(msg tea.KeyPressMsg) (bool, Model) {
	switch {
	case key.Matches(msg, pickerKeys.Accept):
		link, ok := m.picker.selected()
		if !ok {
			// Nothing to pick, so enter goes back to being a newline.
			m.picking = false
			return false, m
		}

		n := m.textarea.Column() - m.anchorCol
		if m.opened {
			n += utf8.RuneCountInString(linkOpener)
		}

		for range n {
			m.textarea, _ = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
		}

		m.textarea.InsertString(link.Text)
		m.picking = false

		return true, m

	case key.Matches(msg, pickerKeys.Cancel):
		m.picking = false
		return true, m

	case key.Matches(msg, pickerKeys.Next):
		m.picker.move(1)
		return true, m

	case key.Matches(msg, pickerKeys.Prev):
		m.picker.move(-1)
		return true, m
	}

	return false, m
}

func (m *Model) openPicker(opened bool) {
	m.picking, m.opened = true, opened
	m.anchorRow, m.anchorCol = m.textarea.Line(), m.textarea.Column()
	m.picker.setQuery("")
}

// followQuery reads the query back out of the note after an edit, and closes
// the picker once the cursor has left it: moved off the line, back before
// where it started, or past a closing bracket typed by hand.
func (m *Model) followQuery() {
	col := m.textarea.Column()
	if m.textarea.Line() != m.anchorRow || col < m.anchorCol {
		m.picking = false
		return
	}

	line := m.line()
	query := string(line[m.anchorCol:min(col, len(line))])

	if strings.Contains(query, "]") {
		m.picking = false
		return
	}

	if query != m.picker.query {
		m.picker.setQuery(query)
	}
}

// line is the text of the line the cursor is on.
func (m Model) line() []rune {
	lines := strings.Split(m.textarea.Value(), "\n")
	if m.textarea.Line() >= len(lines) {
		return nil
	}

	return []rune(lines[m.textarea.Line()])
}

// openerBeforeCursor reports whether the text just before the cursor, on the
// cursor's line, is linkOpener.
func (m Model) openerBeforeCursor() bool {
	line := m.line()
	col := min(m.textarea.Column(), len(line))

	return strings.HasSuffix(string(line[:col]), linkOpener)
}

func (m Model) View() tea.View {
	keys := help.KeyMap(m.keys)
	if m.picking {
		keys = pickerKeys
	}

	content := strings.Join([]string{
		headerStyle.Render(m.header),
		"",
		m.textarea.View(),
		m.help.View(keys),
	}, "\n")

	if m.picking {
		content = m.withPopup(content)
	}

	v := tea.NewView(content)
	v.AltScreen = true

	return v
}

// withPopup draws the picker over content, just below the cursor and lined up
// with where the link will go. Near the bottom of the screen it opens upward
// instead.
func (m Model) withPopup(content string) string {
	// The textarea only reports where its cursor is when it draws a real one,
	// so ask a copy that does.
	ta := m.textarea
	ta.SetVirtualCursor(false)

	cursor := ta.Cursor()
	if cursor == nil || m.width <= 0 || m.height <= 0 {
		return content
	}

	popup := m.picker.View(m.width)
	w, h := lipgloss.Width(popup), lipgloss.Height(popup)

	typed := m.textarea.Column() - m.anchorCol
	if m.opened {
		typed += utf8.RuneCountInString(linkOpener)
	}

	x := max(min(cursor.X-typed, m.width-w), 0)

	y := bodyTop + cursor.Y
	bottom := bodyTop + m.textarea.Height()

	switch {
	case y+1+h <= bottom:
		y++
	case y-h >= bodyTop:
		y -= h
	default:
		y = max(min(y+1, m.height-h), 0)
	}

	canvas := lipgloss.NewCanvas(m.width, m.height)
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(content),
		lipgloss.NewLayer(popup).X(x).Y(y).Z(1),
	))

	return canvas.Render()
}

// Result reports what the user decided.
func (m Model) Result() Result {
	return Result{Body: m.textarea.Value(), Saved: m.saved}
}
