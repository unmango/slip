package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
)

const (
	// popupRows is how many matches the popup shows at once.
	popupRows = 8

	// popupMinWidth and popupMaxWidth bound the popup's inner width, so a
	// short title does not make a sliver and a long one does not cover the
	// note.
	popupMinWidth = 24
	popupMaxWidth = 60
)

var (
	matchStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	markerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true)
	idStyle       = lipgloss.NewStyle().Faint(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	popupStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))
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

// picker is a fuzzy finder over notes. It holds no text of its own: the query
// is whatever the person has typed into the note since the picker opened.
type picker struct {
	links   candidates
	matches fuzzy.Matches
	query   string
	cursor  int
	loaded  bool
	err     error
}

func (p *picker) setLinks(links []Link, err error) {
	p.links, p.err, p.loaded = links, err, true
	p.filter()
}

func (p *picker) setQuery(q string) {
	p.query = q
	p.filter()
}

// selected is the link under the cursor, if anything matched.
func (p picker) selected() (Link, bool) {
	if p.cursor >= len(p.matches) {
		return Link{}, false
	}

	return p.links[p.matches[p.cursor].Index], true
}

// move steps through the matches; positive is toward worse matches.
func (p *picker) move(delta int) {
	p.cursor = max(min(p.cursor+delta, len(p.matches)-1), 0)
}

// filter reruns the query. An empty query keeps every note in the order the
// source gave them.
func (p *picker) filter() {
	p.cursor = 0

	if p.query != "" {
		p.matches = fuzzy.FindFrom(p.query, p.links)
		return
	}

	p.matches = make(fuzzy.Matches, len(p.links))
	for i := range p.links {
		p.matches[i] = fuzzy.Match{Str: p.links.String(i), Index: i}
	}
}

// View renders the popup, best match first, no wider than maxWidth including
// its border.
func (p picker) View(maxWidth int) string {
	// Scroll just far enough to keep the cursor on screen.
	offset := max(p.cursor-popupRows+1, 0)
	visible := p.matches[min(offset, len(p.matches)):min(offset+popupRows, len(p.matches))]

	lines := make([]string, 0, len(visible)+1)
	for i, m := range visible {
		lines = append(lines, p.row(m, offset+i == p.cursor))
	}

	lines = append(lines, p.status())

	inner := popupMinWidth
	for _, l := range lines {
		inner = max(inner, lipgloss.Width(l))
	}

	inner = max(min(inner, popupMaxWidth, maxWidth-2), 1)

	truncate := lipgloss.NewStyle().MaxWidth(inner)
	for i, l := range lines {
		l = truncate.Render(l)
		lines[i] = l + strings.Repeat(" ", max(inner-lipgloss.Width(l), 0))
	}

	return popupStyle.Render(strings.Join(lines, "\n"))
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

	return b.String()
}
