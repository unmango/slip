package tui_test

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/unmango/slip/internal/tui"
)

// typed drives a model the way the runtime would: size it, type into it, then
// press one key. It returns the final model and the command that key produced.
func typed(t *testing.T, body string, final tea.KeyPressMsg) (tui.Model, tea.Cmd) {
	t.Helper()

	var m tea.Model = tui.New("202609081412", "fleeting", "/notes/202609081412.md", nil)

	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	for _, r := range body {
		// Text is what the textarea inserts from; Code alone types nothing.
		msg := tea.KeyPressMsg{Code: r, Text: string(r)}
		if r == '\n' {
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		}

		m, _ = m.Update(msg)
	}

	m, cmd := m.Update(final)

	return m.(tui.Model), cmd
}

func TestSaveKeys(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: 's', Mod: tea.ModCtrl},
		{Code: 'd', Mod: tea.ModCtrl},
	} {
		t.Run(key.String(), func(t *testing.T) {
			m, cmd := typed(t, "a thought", key)

			got := m.Result()
			if !got.Saved {
				t.Error("Saved = false, want true")
			}

			if want := "a thought"; got.Body != want {
				t.Errorf("Body = %q, want %q", got.Body, want)
			}

			assertQuits(t, cmd)
		})
	}
}

func TestDiscardKeys(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: 'c', Mod: tea.ModCtrl},
		{Code: tea.KeyEscape},
	} {
		t.Run(key.String(), func(t *testing.T) {
			m, cmd := typed(t, "a thought", key)

			if m.Result().Saved {
				t.Error("Saved = true, want false")
			}

			assertQuits(t, cmd)
		})
	}
}

// Enter must insert a newline rather than saving: this is a multi-line editor.
func TestEnterIsANewline(t *testing.T) {
	m, _ := typed(t, "first\nsecond", tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

	got := m.Result()
	if want := "first\nsecond"; got.Body != want {
		t.Errorf("Body = %q, want %q", got.Body, want)
	}

	if !got.Saved {
		t.Error("Saved = false, want true")
	}
}

// Nothing typed is not an error here; the caller decides what an empty capture
// means.
func TestEmptyCapture(t *testing.T) {
	m, _ := typed(t, "", tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})

	if got := m.Result(); got.Body != "" {
		t.Errorf("Body = %q, want empty", got.Body)
	}
}

func TestViewShowsHeader(t *testing.T) {
	var m tea.Model = tui.New("202609081412", "fleeting", "/notes/202609081412.md", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	view := m.(tui.Model).View().Content
	for _, want := range []string{"202609081412", "fleeting", "/notes/202609081412.md"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() is missing %q:\n%s", want, view)
		}
	}
}

// The terminal's background decides the palette, so a light answer must reach
// the textarea rather than falling through to it as an ordinary message.
func TestBackgroundColorRestyles(t *testing.T) {
	var m tea.Model = tui.New("202609081412", "fleeting", "/notes/202609081412.md", nil)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	dark := m.(tui.Model).View().Content

	m, _ = m.Update(tea.BackgroundColorMsg{Color: color.White})

	if light := m.(tui.Model).View().Content; light == dark {
		t.Error("View() is unchanged on a light background, want restyled")
	}
}

func assertQuits(t *testing.T, cmd tea.Cmd) {
	t.Helper()

	if cmd == nil {
		t.Fatal("got a nil command, want tea.Quit")
	}

	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("command produced %T, want tea.QuitMsg", cmd())
	}
}
