package store_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/unmango/slip/internal/note"
	"github.com/unmango/slip/internal/store"
)

var now = time.Date(2026, 9, 8, 14, 12, 33, 0, time.UTC)

func TestPath(t *testing.T) {
	s := store.New(afero.NewMemMapFs(), "/notes")

	if got, want := s.Path("202609081412"), filepath.Join("/notes", "202609081412.md"); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestCreateWritesFile(t *testing.T) {
	fsys := afero.NewMemMapFs()
	s := store.New(fsys, "/notes")

	path, err := s.Create(note.New(now, "a thought\n"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if want := filepath.Join("/notes", "202609081412.md"); path != want {
		t.Errorf("Create() = %q, want %q", path, want)
	}

	b, err := afero.ReadFile(fsys, path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}

	got, err := note.Unmarshal(b)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.Body != "a thought\n" {
		t.Errorf("Body = %q, want %q", got.Body, "a thought\n")
	}
}

// The directory is created on first use rather than requiring setup.
func TestCreateMakesDir(t *testing.T) {
	fsys := afero.NewMemMapFs()
	s := store.New(fsys, "/notes/deep/nested")

	if _, err := s.Create(note.New(now, "a thought")); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if ok, err := afero.DirExists(fsys, "/notes/deep/nested"); err != nil || !ok {
		t.Errorf("DirExists() = %v, %v; want true, nil", ok, err)
	}
}

func TestExists(t *testing.T) {
	fsys := afero.NewMemMapFs()
	s := store.New(fsys, "/notes")

	if s.Exists("202609081412") {
		t.Error("Exists() = true before anything was written")
	}

	if _, err := s.Create(note.New(now, "a thought")); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if !s.Exists("202609081412") {
		t.Error("Exists() = false after Create")
	}
}

// Two captures in the same minute must produce two notes, and each note's id
// must match its filename or the correspondence zettel_id guarantees is broken.
func TestCreateResolvesCollision(t *testing.T) {
	fsys := afero.NewMemMapFs()
	s := store.New(fsys, "/notes")

	first, err := s.Create(note.New(now, "first thought"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	second, err := s.Create(note.New(now, "second thought"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if first == second {
		t.Fatalf("Create() returned %q twice", first)
	}

	if want := filepath.Join("/notes", "202609081412.md"); first != want {
		t.Errorf("first = %q, want %q", first, want)
	}

	if want := filepath.Join("/notes", "202609081412a.md"); second != want {
		t.Errorf("second = %q, want %q", second, want)
	}

	for _, path := range []string{first, second} {
		b, err := afero.ReadFile(fsys, path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}

		n, err := note.Unmarshal(b)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}

		stem := strings.TrimSuffix(filepath.Base(path), ".md")
		if n.ZettelID != stem {
			t.Errorf("%s has zettel_id %q, want %q", path, n.ZettelID, stem)
		}
	}
}

// A note already at the target path is never overwritten, even if the caller
// insists on the id.
func TestCreateRefusesOverwrite(t *testing.T) {
	fsys := afero.NewMemMapFs()
	s := store.New(fsys, "/notes")

	path := filepath.Join("/notes", "202609081412.md")
	if err := afero.WriteFile(fsys, path, []byte("existing\n"), 0o644); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if _, err := s.Create(note.New(now, "a thought")); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	b, err := afero.ReadFile(fsys, path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}

	if string(b) != "existing\n" {
		t.Errorf("existing note was overwritten: %q", b)
	}
}

// Nothing about a stored note may be lost on the way to disk.
func TestCreateRoundTrips(t *testing.T) {
	fsys := afero.NewMemMapFs()
	s := store.New(fsys, "/notes")

	want := note.New(now, "a thought\n\nwith a body\n")
	want.Tags = []string{"systems", "reading"}
	want.SourceURI = "https://example.com/a"

	path, err := s.Create(want)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	b, err := afero.ReadFile(fsys, path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}

	got, err := note.Unmarshal(b)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.Title != want.Title || got.Body != want.Body || got.SourceURI != want.SourceURI {
		t.Errorf("round trip lost data: got %+v, want %+v", got, want)
	}
}
