package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
)

// pickerChrome is the count line and the prompt under the results.
const pickerChrome = 2

var (
	matchStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	markerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true)
	idStyle       = lipgloss.NewStyle().Faint(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

// linksLoadedMsg carries what a LinkSource produced.
type linksLoadedMsg struct {
	links []Link
	err   error
}

func (src LinkSource) load() tea.Cmd {
	return func() tea.Msg {
		links, err := src()
		return linksLoadedMsg{links: links, err: err}
	}
}

// candidates is what the query is matched against: the title, then the id, so
// a remembered zettel id finds its note too.
type candidates []Link

func (c candidates) String(i int) string { return c[i].Title + " " + c[i].ID }
func (c candidates) Len() int            { return len(c) }

// picker is a fuzzy finder over notes, laid out the way fzf lays itself out by
// default: the prompt at the bottom and the best match directly above it.
type picker struct {
	query   textinput.Model
	links   candidates
	matches fuzzy.Matches
	cursor  int
	width   int
	height  int
	loaded  bool
	err     error
}

func newPicker() picker {
	q := textinput.New()
	q.Prompt = "> "

	return picker{query: q}
}

func (p *picker) setLinks(links []Link, err error) {
	p.links, p.err, p.loaded = links, err, true
	p.filter()
}

func (p *picker) setSize(width, height int) {
	p.width, p.height = width, height
	p.query.SetWidth(max(width-lipgloss.Width(p.query.Prompt)-1, 1))
}

func (p *picker) setStyles(isDark bool) {
	p.query.SetStyles(textinput.DefaultStyles(isDark))
}

// open starts a fresh search.
func (p *picker) open() tea.Cmd {
	p.query.Reset()
	p.filter()

	return p.query.Focus()
}

func (p *picker) close() {
	p.query.Blur()
}

// selected is the link under the cursor, if anything matched.
func (p picker) selected() (Link, bool) {
	if p.cursor >= len(p.matches) {
		return Link{}, false
	}

	return p.links[p.matches[p.cursor].Index], true
}

// move steps through the matches; positive is toward worse matches, which is
// up the screen.
func (p *picker) move(delta int) {
	p.cursor = max(min(p.cursor+delta, len(p.matches)-1), 0)
}

func (p picker) Update(msg tea.Msg) (picker, tea.Cmd) {
	before := p.query.Value()

	var cmd tea.Cmd
	p.query, cmd = p.query.Update(msg)

	if p.query.Value() != before {
		p.filter()
	}

	return p, cmd
}

// filter reruns the query. An empty query keeps every note in the order the
// source gave them.
func (p *picker) filter() {
	p.cursor = 0

	if q := p.query.Value(); q != "" {
		p.matches = fuzzy.FindFrom(q, p.links)
		return
	}

	p.matches = make(fuzzy.Matches, len(p.links))
	for i := range p.links {
		p.matches[i] = fuzzy.Match{Str: p.links.String(i), Index: i}
	}
}

func (p picker) View() string {
	rows := max(p.height-pickerChrome, 0)

	// Scroll just far enough to keep the cursor on screen.
	offset := max(p.cursor-rows+1, 0)
	visible := p.matches[min(offset, len(p.matches)):min(offset+rows, len(p.matches))]

	lines := make([]string, 0, p.height)
	for range rows - len(visible) {
		lines = append(lines, "")
	}

	for i, m := range slices.Backward(visible) {
		lines = append(lines, p.row(m, offset+i == p.cursor))
	}

	return strings.Join(append(lines, p.status(), p.query.View()), "\n")
}

func (p picker) status() string {
	switch {
	case p.err != nil:
		return errorStyle.Render("  " + p.err.Error())
	case !p.loaded:
		return idStyle.Render("  loading…")
	default:
		return idStyle.Render(fmt.Sprintf("  %d/%d", len(p.matches), len(p.links)))
	}
}

// row renders one match with the matched characters highlighted. The id is
// dimmed, since the title is what a person reads.
func (p picker) row(m fuzzy.Match, selected bool) string {
	var b strings.Builder

	if selected {
		b.WriteString(markerStyle.Render("> "))
	} else {
		b.WriteString("  ")
	}

	base := lipgloss.NewStyle()
	if selected {
		base = selectedStyle
	}

	// Runs of characters that share a style are rendered together, so the
	// output carries one escape sequence per run rather than per character.
	type span struct{ id, match bool }

	var (
		run     strings.Builder
		current span
		title   = len(p.links[m.Index].Title)
		matched = m.MatchedIndexes
	)

	flush := func() {
		style := base
		if current.id {
			style = idStyle
		}

		if current.match {
			style = style.Inherit(matchStyle)
		}

		b.WriteString(style.Render(run.String()))
		run.Reset()
	}

	for i, r := range m.Str {
		next := span{id: i >= title}
		if len(matched) > 0 && matched[0] == i {
			next.match = true
			matched = matched[1:]
		}

		if next != current && run.Len() > 0 {
			flush()
		}

		current = next
		run.WriteRune(r)
	}

	flush()

	if p.width <= 0 {
		return b.String()
	}

	return lipgloss.NewStyle().MaxWidth(p.width).Render(b.String())
}
