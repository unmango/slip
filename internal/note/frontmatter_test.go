package note_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	notev1 "github.com/unmango/slip/gen/unmango/zettelkasten/note/v1alpha1"
	"github.com/unmango/slip/internal/note"
)

func TestMarshalGolden(t *testing.T) {
	n := note.New(
		time.Date(2026, 9, 8, 14, 12, 33, 0, time.FixedZone("CDT", -5*60*60)),
		"The thing I was thinking about\n\nand the rest of what I typed, verbatim.\n",
	)

	got, err := note.Marshal(n)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	golden := filepath.Join("testdata", "basic.md")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatalf("writing golden: %v", err)
		}
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("reading golden: %v", err)
	}

	if string(got) != string(want) {
		t.Errorf("Marshal() mismatch with %s\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
	}
}

// A bare 202609081412 is a YAML integer. Losing the string type would break the
// filename/frontmatter correspondence that zettel_id exists to guarantee.
func TestMarshalQuotesZettelID(t *testing.T) {
	n := note.New(time.Date(2026, 9, 8, 14, 12, 33, 0, time.UTC), "x")

	got, err := note.Marshal(n)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	if !strings.Contains(string(got), `zettel_id: "202609081412"`) {
		t.Errorf("zettel_id is not quoted:\n%s", got)
	}
}

func TestRoundTrip(t *testing.T) {
	now := time.Date(2026, 9, 8, 14, 12, 33, 0, time.FixedZone("CDT", -5*60*60))

	tests := map[string]note.Note{
		"minimal": note.New(now, "a thought"),
		"empty body": {
			ZettelID:   "202609081412",
			Type:       notev1.NoteType_NOTE_TYPE_FLEETING,
			Format:     notev1.ContentFormat_CONTENT_FORMAT_MARKDOWN,
			CreateTime: now,
		},
		"unicode":            note.New(now, "思考\n\némoji 🧠 and ümlauts\n"),
		"body with fence":    note.New(now, "a thought\n\n---\n\nnot frontmatter\n"),
		"body leading blank": note.New(now, "\n\nleading blank lines\n"),
		"trailing newlines":  note.New(now, "a thought\n\n\n"),
		"populated": {
			ZettelID:    "202609081412",
			Title:       "a thought",
			Body:        "a thought\n",
			Type:        notev1.NoteType_NOTE_TYPE_LITERATURE,
			Format:      notev1.ContentFormat_CONTENT_FORMAT_ORG,
			Tags:        []string{"reading", "systems"},
			SourceURI:   "https://example.com/a",
			DisplayName: "A Thought",
			Labels:      map[string]string{"stage": "inbox"},
			Annotations: map[string]string{"note": "captured on the train"},
			CreateTime:  now,
		},
	}

	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			b, err := note.Marshal(want)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			got, err := note.Unmarshal(b)
			if err != nil {
				t.Fatalf("Unmarshal() error = %v\n%s", err, b)
			}

			assertEqual(t, got, want)
		})
	}
}

func TestUnmarshalErrors(t *testing.T) {
	tests := map[string]string{
		"no frontmatter": "just a body\n",
		"unterminated":   "---\nzettel_id: \"1\"\n",
		"unknown type":   "---\nnote_type: NOTE_TYPE_NONSENSE\n---\n\nbody\n",
		"unknown format": "---\nformat: CONTENT_FORMAT_NONSENSE\n---\n\nbody\n",
		"malformed yaml": "---\n\tbad: [\n---\n\nbody\n",
		"empty file":     "",
	}

	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := note.Unmarshal([]byte(in)); err == nil {
				t.Errorf("Unmarshal(%q) = nil error, want an error", in)
			}
		})
	}
}

// An absent enum is the unspecified zero value rather than an error, so a
// hand-written note need not spell out every field.
func TestUnmarshalOmittedEnums(t *testing.T) {
	got, err := note.Unmarshal([]byte("---\nzettel_id: \"202609081412\"\ntitle: a thought\n---\n\na thought\n"))
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.Type != notev1.NoteType_NOTE_TYPE_UNSPECIFIED {
		t.Errorf("Type = %v, want unspecified", got.Type)
	}

	if got.Format != notev1.ContentFormat_CONTENT_FORMAT_UNSPECIFIED {
		t.Errorf("Format = %v, want unspecified", got.Format)
	}
}

func assertEqual(t *testing.T, got, want note.Note) {
	t.Helper()

	if got.ZettelID != want.ZettelID {
		t.Errorf("ZettelID = %q, want %q", got.ZettelID, want.ZettelID)
	}

	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}

	if got.Body != want.Body {
		t.Errorf("Body = %q, want %q", got.Body, want.Body)
	}

	if got.Type != want.Type {
		t.Errorf("Type = %v, want %v", got.Type, want.Type)
	}

	if got.Format != want.Format {
		t.Errorf("Format = %v, want %v", got.Format, want.Format)
	}

	if !slices.Equal(got.Tags, want.Tags) {
		t.Errorf("Tags = %v, want %v", got.Tags, want.Tags)
	}

	if got.SourceURI != want.SourceURI {
		t.Errorf("SourceURI = %q, want %q", got.SourceURI, want.SourceURI)
	}

	if got.DisplayName != want.DisplayName {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, want.DisplayName)
	}

	if !maps.Equal(got.Labels, want.Labels) {
		t.Errorf("Labels = %v, want %v", got.Labels, want.Labels)
	}

	if !maps.Equal(got.Annotations, want.Annotations) {
		t.Errorf("Annotations = %v, want %v", got.Annotations, want.Annotations)
	}

	if !got.CreateTime.Equal(want.CreateTime) {
		t.Errorf("CreateTime = %v, want %v", got.CreateTime, want.CreateTime)
	}
}
