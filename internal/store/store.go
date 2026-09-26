// Package store writes notes to a directory. It knows about files and nothing
// about their format.
package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
	"github.com/unmango/slip/internal/note"
)

// ext is the file extension zk indexes by default.
const ext = ".md"

// Store is a flat directory of notes. Flat is deliberate: a zettelkasten has no
// hierarchy, and it keeps grep, fzf, and zk list trivial.
type Store struct {
	fs  afero.Fs
	Dir string
}

func New(fsys afero.Fs, dir string) *Store {
	return &Store{fs: fsys, Dir: dir}
}

// Path is where the note with this id lives. There is no slug: titles change,
// and a slug in the filename rots the moment one does.
func (s *Store) Path(zettelID string) string {
	return filepath.Join(s.Dir, zettelID+ext)
}

// Exists reports whether a note already occupies this id.
func (s *Store) Exists(zettelID string) bool {
	ok, err := afero.Exists(s.fs, s.Path(zettelID))
	return err == nil && ok
}

// Create writes n, resolving an id collision first. Two thoughts captured in
// the same minute share a timestamp, which is routine when pasting, so the id
// in the frontmatter is updated alongside the filename to keep them in step.
func (s *Store) Create(n note.Note) (string, error) {
	if err := s.fs.MkdirAll(s.Dir, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", s.Dir, err)
	}

	n.ZettelID = note.NextID(n.ZettelID, s.Exists)

	b, err := note.Marshal(n)
	if err != nil {
		return "", err
	}

	path := s.Path(n.ZettelID)

	// O_EXCL rather than a plain create: NextID raced against anything else
	// writing to this directory, and losing that race must not clobber a note.
	f, err := s.fs.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("creating %s: %w", path, err)
	}

	if _, err := f.Write(b); err != nil {
		f.Close()
		// A half-written note is worse than none: it would index as a real
		// note and read as a truncated thought.
		_ = s.fs.Remove(path)

		return "", fmt.Errorf("writing %s: %w", path, err)
	}

	if err := f.Close(); err != nil {
		return "", fmt.Errorf("closing %s: %w", path, err)
	}

	return path, nil
}
