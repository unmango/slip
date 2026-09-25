package config_test

import (
	"path/filepath"
	"testing"

	"github.com/UnstoppableMango/zettelkasten/internal/config"
	"github.com/UnstoppableMango/zettelkasten/internal/notebook"
	"github.com/spf13/afero"
)

func TestResolvePrecedence(t *testing.T) {
	tests := map[string]struct {
		flagDir  string
		env      map[string]string
		markers  []string
		cwd      string
		want     string
		notebook bool
	}{
		"flag wins over everything": {
			flagDir: "/flag",
			env:     map[string]string{config.EnvDir: "/env", "XDG_DATA_HOME": "/xdg"},
			markers: []string{"/cwd/.zk"},
			cwd:     "/cwd",
			want:    "/flag",
		},
		"env wins over notebook": {
			env:     map[string]string{config.EnvDir: "/env", "XDG_DATA_HOME": "/xdg"},
			markers: []string{"/cwd/.zk"},
			cwd:     "/cwd",
			want:    "/env",
		},
		"env wins over notebook env": {
			env:     map[string]string{config.EnvDir: "/env", notebook.EnvDir: "/notebook-env"},
			markers: []string{"/cwd/.zk"},
			cwd:     "/cwd",
			want:    "/env",
		},
		"notebook env wins over walk-up": {
			env:      map[string]string{notebook.EnvDir: "/notebook-env", "XDG_DATA_HOME": "/xdg"},
			markers:  []string{"/cwd/.zk", "/notebook-env/.zk"},
			cwd:      "/cwd",
			want:     "/notebook-env",
			notebook: true,
		},
		// zk refuses a ZK_NOTEBOOK_DIR it has not initialised, so notes still
		// go there but nothing may assume zk can read it.
		"notebook env without a marker is not a notebook": {
			env:     map[string]string{notebook.EnvDir: "/notebook-env", "XDG_DATA_HOME": "/xdg"},
			markers: []string{"/cwd/.zk"},
			cwd:     "/cwd",
			want:    "/notebook-env",
		},
		"notebook wins over xdg": {
			env:      map[string]string{"XDG_DATA_HOME": "/xdg"},
			markers:  []string{"/cwd/.zk"},
			cwd:      "/cwd",
			want:     "/cwd",
			notebook: true,
		},
		"notebook found by walking up": {
			env:      map[string]string{"XDG_DATA_HOME": "/xdg"},
			markers:  []string{"/cwd/.zk"},
			cwd:      "/cwd/deep/nested",
			want:     "/cwd",
			notebook: true,
		},
		"xdg when no notebook": {
			env:  map[string]string{"XDG_DATA_HOME": "/xdg"},
			cwd:  "/cwd",
			want: filepath.Join("/xdg", "zettelkasten"),
		},
		"home when no xdg": {
			env:  map[string]string{"HOME": "/home/someone"},
			cwd:  "/cwd",
			want: filepath.Join("/home/someone", ".local", "share", "zettelkasten"),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// Clear everything the resolver reads, then set only what this case
			// declares, so an inherited variable cannot change the outcome.
			for _, k := range []string{config.EnvDir, notebook.EnvDir, "XDG_DATA_HOME", "HOME"} {
				t.Setenv(k, "")
			}

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			fsys := afero.NewMemMapFs()
			for _, m := range tt.markers {
				if err := fsys.MkdirAll(m, 0o755); err != nil {
					t.Fatalf("seeding %s: %v", m, err)
				}
			}

			got, err := config.Resolve(fsys, tt.cwd, tt.flagDir)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}

			if got.Dir != tt.want {
				t.Errorf("Dir = %q, want %q", got.Dir, tt.want)
			}

			if got.Notebook != tt.notebook {
				t.Errorf("Notebook = %v, want %v", got.Notebook, tt.notebook)
			}
		})
	}
}

// Without HOME or XDG_DATA_HOME there is nowhere sensible to put notes, and
// guessing would scatter them.
func TestResolveNoHome(t *testing.T) {
	for _, k := range []string{config.EnvDir, notebook.EnvDir, "XDG_DATA_HOME", "HOME"} {
		t.Setenv(k, "")
	}

	if _, err := config.Resolve(afero.NewMemMapFs(), "/cwd", ""); err == nil {
		t.Error("Resolve() = nil error, want an error")
	}
}

// An explicit directory that happens to be a notebook is still reported as one,
// so the caller can say so.
func TestResolveFlagDirIsNotebook(t *testing.T) {
	for _, k := range []string{config.EnvDir, notebook.EnvDir, "XDG_DATA_HOME", "HOME"} {
		t.Setenv(k, "")
	}

	fsys := afero.NewMemMapFs()
	if err := fsys.MkdirAll("/flag/.zk", 0o755); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	got, err := config.Resolve(fsys, "/cwd", "/flag")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if !got.Notebook {
		t.Error("Notebook = false, want true")
	}
}
