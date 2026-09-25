// Package config resolves where notes are written.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/unmango/slip/internal/notebook"
	"github.com/spf13/afero"
)

// EnvDir points slip at a notes directory directly.
const EnvDir = "ZK_DIR"

// dirName is the leaf under the XDG data directory.
const dirName = "zettelkasten"

// Config is the resolved runtime configuration. There is no config file: one
// directory does not justify the surface.
type Config struct {
	// Dir is where notes are written.
	Dir string

	// Notebook reports whether Dir is a zk notebook, which is what makes the
	// interop behaviour worth mentioning to the user.
	Notebook bool
}

// Resolve picks the notes directory. The first non-empty source wins: the
// explicit flag, $ZK_DIR, a discovered zk notebook, then the XDG data path.
func Resolve(fsys afero.Fs, cwd, flagDir string) (Config, error) {
	if flagDir != "" {
		root, ok := notebook.Find(fsys, flagDir)
		return Config{Dir: flagDir, Notebook: ok && root == flagDir}, nil
	}

	if dir := os.Getenv(EnvDir); dir != "" {
		root, ok := notebook.Find(fsys, dir)
		return Config{Dir: dir, Notebook: ok && root == dir}, nil
	}

	// $ZK_NOTEBOOK_DIR is taken on trust as the destination, but zk will not
	// read it until it has been initialised, so only a marker makes it a
	// notebook.
	if dir, ok := notebook.Dir(fsys, cwd); ok {
		root, found := notebook.Find(fsys, dir)
		return Config{Dir: dir, Notebook: found && root == dir}, nil
	}

	dir, err := dataDir()
	if err != nil {
		return Config{}, err
	}

	return Config{Dir: dir}, nil
}

// dataDir is the XDG fallback, spelled out rather than taking a dependency for
// two environment variables.
func dataDir() (string, error) {
	if data := os.Getenv("XDG_DATA_HOME"); data != "" {
		return filepath.Join(data, dirName), nil
	}

	home := os.Getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("cannot resolve a notes directory: set $%s, $XDG_DATA_HOME, or $HOME", EnvDir)
	}

	return filepath.Join(home, ".local", "share", dirName), nil
}
