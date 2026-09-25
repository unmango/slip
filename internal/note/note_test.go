package note_test

import (
	"testing"
	"time"

	notev1 "github.com/unmango/slip/gen/unmango/zettelkasten/note/v1alpha1"
	"github.com/unmango/slip/internal/note"
)

func TestTitleFrom(t *testing.T) {
	tests := map[string]struct {
		body string
		want string
	}{
		"single line":      {"a thought", "a thought"},
		"first of many":    {"a thought\nand more\n", "a thought"},
		"leading blanks":   {"\n\n  \na thought\n", "a thought"},
		"atx heading":      {"# a thought\n", "a thought"},
		"deep heading":     {"### a thought\n", "a thought"},
		"heading no space": {"#a thought\n", "a thought"},
		"crlf":             {"a thought\r\nmore\r\n", "a thought"},
		"colon":            {"a thought: with a colon\n", "a thought: with a colon"},
		"indented":         {"    a thought\n", "a thought"},
		"empty":            {"", ""},
		"whitespace only":  {"   \n\t\n", ""},
		"hashes only":      {"###\n", ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := note.TitleFrom(tt.body); got != tt.want {
				t.Errorf("TitleFrom(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	now := time.Date(2026, 9, 8, 14, 12, 33, 0, time.UTC)
	n := note.New(now, "a thought\nand more")

	if got, want := n.ZettelID, "202609081412"; got != want {
		t.Errorf("ZettelID = %q, want %q", got, want)
	}

	if got, want := n.Title, "a thought"; got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}

	// The captured text is stored exactly as typed; the title is derived from
	// it, not cut out of it.
	if got, want := n.Body, "a thought\nand more"; got != want {
		t.Errorf("Body = %q, want %q", got, want)
	}

	if got, want := n.Type, notev1.NoteType_NOTE_TYPE_FLEETING; got != want {
		t.Errorf("Type = %v, want %v", got, want)
	}

	if got, want := n.Format, notev1.ContentFormat_CONTENT_FORMAT_MARKDOWN; got != want {
		t.Errorf("Format = %v, want %v", got, want)
	}

	if !n.CreateTime.Equal(now) {
		t.Errorf("CreateTime = %v, want %v", n.CreateTime, now)
	}
}

func TestName(t *testing.T) {
	n := note.Note{ZettelID: "202609081412"}
	if got, want := n.Name(), "notes/202609081412"; got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

// A hand-captured thought does not have meaningful sub-second precision, and
// the timestamp is written into a file people read.
func TestNewTruncatesToSecond(t *testing.T) {
	now := time.Date(2026, 9, 8, 14, 12, 33, 362153402, time.UTC)

	got := note.New(now, "a thought").CreateTime
	if want := time.Date(2026, 9, 8, 14, 12, 33, 0, time.UTC); !got.Equal(want) {
		t.Errorf("CreateTime = %v, want %v", got, want)
	}
}

// A line that is only heading markers carries no title, but the thought below
// it still does. Stopping at the marker would drop a title the note has.
func TestTitleFromSkipsEmptyHeadings(t *testing.T) {
	tests := map[string]struct {
		body string
		want string
	}{
		"bare hashes then content":    {"###\nreal content\n", "real content"},
		"hashes, blank, content":      {"###\n\nreal content\n", "real content"},
		"hash and spaces":             {"#   \nreal content\n", "real content"},
		"several empty headings":      {"#\n##\n###\nreal content\n", "real content"},
		"still empty when no content": {"###\n\n   \n", ""},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := note.TitleFrom(tt.body); got != tt.want {
				t.Errorf("TitleFrom(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}
