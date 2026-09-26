package zk_test

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/unmango/slip/internal/zk"
)

// The fixture is real output from `zk list --format jsonl` over notes slip
// wrote. zk's JSON carries no version and promises no compatibility, so this is
// the only thing standing between a zk upgrade and a silent breakage.
func fixture(t *testing.T) []zk.Note {
	t.Helper()

	f, err := os.Open(filepath.Join("testdata", "list.jsonl"))
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	notes, err := zk.ParseList(f)
	if err != nil {
		t.Fatalf("ParseList() error = %v", err)
	}

	return notes
}

func TestParseList(t *testing.T) {
	notes := fixture(t)

	if len(notes) != 2 {
		t.Fatalf("got %d notes, want 2", len(notes))
	}

	// Newest first, which is the order zk lists in.
	got := notes[1]

	if want := "202609090106.md"; got.Filename != want {
		t.Errorf("Filename = %q, want %q", got.Filename, want)
	}

	if want := "a thought about interop #systems"; got.Title != want {
		t.Errorf("Title = %q, want %q", got.Title, want)
	}

	if !strings.Contains(got.Body, "It links to") {
		t.Errorf("Body = %q, want it to contain the link line", got.Body)
	}

	if got.WordCount == 0 {
		t.Error("WordCount = 0, want zk's count")
	}

	if got.Checksum == "" {
		t.Error("Checksum is empty")
	}
}

// zk parses tags out of the body for us, from all the syntaxes it supports.
// This is the parser slip does not have to write.
func TestParseListTags(t *testing.T) {
	notes := fixture(t)

	tags := slices.Clone(notes[0].Tags)
	slices.Sort(tags)

	if want := []string{"reading", "systems"}; !slices.Equal(tags, want) {
		t.Errorf("Tags = %v, want %v", tags, want)
	}
}

// The creation-date-key setting slip init writes is what makes zk read our
// timestamp instead of falling back to the filesystem. If this breaks, the
// interop contract is broken.
func TestParseListCreatedMatchesFrontmatter(t *testing.T) {
	for _, n := range fixture(t) {
		raw, ok := n.Metadata["create_time"].(string)
		if !ok {
			t.Fatalf("%s has no create_time in metadata: %v", n.Filename, n.Metadata)
		}

		want, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			t.Fatalf("parsing %q: %v", raw, err)
		}

		if !n.Created.Equal(want) {
			t.Errorf("%s: Created = %v, want %v (from create_time)", n.Filename, n.Created, want)
		}
	}
}

// Frontmatter keys zk does not itself understand must survive verbatim, which
// is what lets slip's schema ride along inside a zk notebook.
func TestParseListPreservesSlipMetadata(t *testing.T) {
	for _, n := range fixture(t) {
		if got := n.ZettelID(); got != n.FilenameStem {
			t.Errorf("%s: ZettelID() = %q, want %q", n.Filename, got, n.FilenameStem)
		}

		for _, key := range []string{"note_type", "format"} {
			if _, ok := n.Metadata[key]; !ok {
				t.Errorf("%s: metadata is missing %q: %v", n.Filename, key, n.Metadata)
			}
		}

		if got := n.Metadata["note_type"]; got != "NOTE_TYPE_FLEETING" {
			t.Errorf("%s: note_type = %v, want NOTE_TYPE_FLEETING", n.Filename, got)
		}
	}
}

func TestParseListEmpty(t *testing.T) {
	notes, err := zk.ParseList(strings.NewReader(""))
	if err != nil {
		t.Fatalf("ParseList() error = %v", err)
	}

	if len(notes) != 0 {
		t.Errorf("got %d notes, want 0", len(notes))
	}
}

// Blank lines are not an error; a truncated object is.
func TestParseListMalformed(t *testing.T) {
	if _, err := zk.ParseList(strings.NewReader("{\"filename\":\n")); err == nil {
		t.Error("ParseList() = nil error, want an error")
	}
}

// zk reports config parse failures on stderr. Without it the wrapped error is
// just an exit status, which says nothing about what went wrong.
func TestListSurfacesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stub is a shell script")
	}

	bin := t.TempDir()
	stub := filepath.Join(bin, zk.Binary)
	script := "#!/bin/sh\necho 'toml: line 3: expected key separator' >&2\nexit 1\n"

	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatalf("writing stub: %v", err)
	}

	t.Setenv("PATH", bin)

	_, err := zk.List(t.Context(), t.TempDir())
	if err == nil {
		t.Fatal("List() = nil error, want an error")
	}

	if !strings.Contains(err.Error(), "expected key separator") {
		t.Errorf("error %q does not carry zk's stderr", err)
	}
}
