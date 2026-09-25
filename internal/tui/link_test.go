package tui_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/UnstoppableMango/zettelkasten/internal/tui"
)

var notes = []tui.Link{
	{ID: "202609090107", Title: "a second thought", Text: "[[202609090107]]"},
	{ID: "202609090106", Title: "a thought about interop", Text: "[[202609090106]]"},
	{ID: "202609090105", Title: "reading list", Text: "[[202609090105]]"},
}

var (
	ctrlL  = tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl}
	ctrlC  = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	ctrlS  = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	enter  = tea.KeyPressMsg{Code: tea.KeyEnter}
	escape = tea.KeyPressMsg{Code: tea.KeyEscape}
	up     = tea.KeyPressMsg{Code: tea.KeyUp}
)

func fixed(links []tui.Link, err error) tui.LinkSource {
	return func() ([]tui.Link, error) { return links, err }
}

// sized builds a capture screen with src and gives it a window, without
// running Init, so nothing has loaded yet.
func sized(src tui.LinkSource) tea.Model {
	var m tea.Model = tui.New("202609081412", "fleeting", "/notes/202609081412.md", src)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	return m
}

// started is sized plus whatever Init produces, which is how the note list
// arrives.
func started(t *testing.T, src tui.LinkSource) tea.Model {
	t.Helper()

	m := sized(src)

	return deliver(m, m.Init())
}

// deliver runs cmd and feeds what it produces back into m, one level deep.
// Commands those messages return are dropped: the cursor blink would tick
// forever.
func deliver(m tea.Model, cmd tea.Cmd) tea.Model {
	if cmd == nil {
		return m
	}

	msg := cmd()

	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		m, _ = m.Update(msg)
		return m
	}

	for _, c := range batch {
		if c == nil {
			continue
		}

		m, _ = m.Update(c())
	}

	return m
}

// press types text then sends each key in turn, returning the final model and
// the command the last key produced.
func press(m tea.Model, text string, keys ...tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	for _, r := range text {
		m, cmd = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	for _, k := range keys {
		m, cmd = m.Update(k)
	}

	return m, cmd
}

func body(m tea.Model) string {
	return m.(tui.Model).Result().Body
}

func view(m tea.Model) string {
	return m.(tui.Model).View().Content
}

func TestBracketsOpenThePickerAndAreReplaced(t *testing.T) {
	m := started(t, fixed(notes, nil))

	m, _ = press(m, "see [[interop", enter)

	if got, want := body(m), "see [[202609090106]]"; got != want {
		t.Errorf("Body = %q, want %q", got, want)
	}
}

func TestCtrlLInsertsAtTheCursor(t *testing.T) {
	m := started(t, fixed(notes, nil))

	m, _ = press(m, "see ")
	m, _ = press(m, "", ctrlL, enter)

	if got, want := body(m), "see [[202609090107]]"; got != want {
		t.Errorf("Body = %q, want %q", got, want)
	}
}

func TestPickerMovesToTheNextMatch(t *testing.T) {
	m := started(t, fixed(notes, nil))

	m, _ = press(m, "", ctrlL, up, enter)

	if got, want := body(m), "[[202609090106]]"; got != want {
		t.Errorf("Body = %q, want %q", got, want)
	}
}

func TestFuzzyRanking(t *testing.T) {
	m := started(t, fixed(notes, nil))

	m, _ = press(m, "[[intrp", enter)

	if got, want := body(m), "[[202609090106]]"; got != want {
		t.Errorf("Body = %q, want %q", got, want)
	}
}

// Leaving the picker is not leaving the note: the keys that discard a capture
// only close the picker while it is open.
func TestCancellingThePickerKeepsTheNote(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{escape, ctrlC} {
		t.Run(key.String(), func(t *testing.T) {
			m := started(t, fixed(notes, nil))

			m, cmd := press(m, "see [[", key)

			if cmd != nil {
				if _, ok := cmd().(tea.QuitMsg); ok {
					t.Fatal("cancelling the picker quit the capture screen")
				}
			}

			if got, want := body(m), "see [["; got != want {
				t.Errorf("Body = %q, want %q", got, want)
			}

			// Typing goes back to the note.
			m, _ = press(m, "x")
			if got, want := body(m), "see [[x"; got != want {
				t.Errorf("Body = %q, want %q", got, want)
			}
		})
	}
}

func TestSaveIsSwallowedWhileThePickerIsOpen(t *testing.T) {
	m := started(t, fixed(notes, nil))

	m, cmd := press(m, "", ctrlL, ctrlS)

	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("ctrl+s in the picker quit the capture screen")
		}
	}

	if m.(tui.Model).Result().Saved {
		t.Error("Saved = true, want false")
	}
}

func TestPickerShowsMatches(t *testing.T) {
	m := started(t, fixed(notes, nil))

	m, _ = press(m, "", ctrlL)

	v := view(m)
	for _, want := range []string{"a second thought", "a thought about interop", "reading list", "3/3"} {
		if !strings.Contains(v, want) {
			t.Errorf("View() is missing %q:\n%s", want, v)
		}
	}

	m, _ = press(m, "reading")

	v = view(m)
	if !strings.Contains(v, "1/3") {
		t.Errorf("View() is missing the count 1/3:\n%s", v)
	}

	if strings.Contains(v, "interop") {
		t.Errorf("View() still lists a note the query excludes:\n%s", v)
	}
}

func TestPickerLoadingThenLoaded(t *testing.T) {
	m := sized(fixed(notes, nil))
	init := m.Init()

	m, _ = press(m, "", ctrlL)
	if v := view(m); !strings.Contains(v, "loading") {
		t.Errorf("View() before the notes arrive is missing \"loading\":\n%s", v)
	}

	m = deliver(m, init)
	if v := view(m); !strings.Contains(v, "a thought about interop") {
		t.Errorf("View() after the notes arrive is missing them:\n%s", v)
	}
}

func TestLoadErrorIsShownAndCaptureStillSaves(t *testing.T) {
	m := started(t, fixed(nil, errors.New("zk exploded")))

	m, _ = press(m, "a thought [[")
	if v := view(m); !strings.Contains(v, "zk exploded") {
		t.Errorf("View() is missing the load error:\n%s", v)
	}

	m, _ = press(m, "", escape, ctrlS)

	got := m.(tui.Model).Result()
	if !got.Saved {
		t.Error("Saved = false, want true")
	}

	if want := "a thought [["; got.Body != want {
		t.Errorf("Body = %q, want %q", got.Body, want)
	}
}

// Outside a notebook there is nothing to link to, so brackets are just text.
func TestNoSourceMeansNoPicker(t *testing.T) {
	m := started(t, nil)

	if v := view(m); strings.Contains(v, "link") {
		t.Errorf("View() offers linking with no source:\n%s", v)
	}

	m, _ = press(m, "see [[x", ctrlL)
	m, _ = press(m, "y")

	if got, want := body(m), "see [[xy"; got != want {
		t.Errorf("Body = %q, want %q", got, want)
	}
}

func TestHelpOffersLinkWithASource(t *testing.T) {
	m := started(t, fixed(notes, nil))

	if v := view(m); !strings.Contains(v, "link") {
		t.Errorf("View() is missing the link binding:\n%s", v)
	}
}

// Pasted text arrives whole, not as key presses, so pasting brackets must not
// open the picker.
func TestPastedBracketsDoNotOpenThePicker(t *testing.T) {
	m := started(t, fixed(notes, nil))

	m, _ = m.Update(tea.PasteMsg{Content: "see [["})
	m, _ = press(m, "x")

	if got, want := body(m), "see [[x"; got != want {
		t.Errorf("Body = %q, want %q", got, want)
	}
}

// Only typing the second bracket opens the picker. Moving the cursor back
// past brackets already in the note must not.
func TestMovingPastBracketsDoesNotOpenThePicker(t *testing.T) {
	m := started(t, fixed(notes, nil))

	m, _ = m.Update(tea.PasteMsg{Content: "[[x"})
	m, _ = press(m, "", tea.KeyPressMsg{Code: tea.KeyLeft})
	m, _ = press(m, "y")

	if got, want := body(m), "[[yx"; got != want {
		t.Errorf("Body = %q, want %q", got, want)
	}
}
