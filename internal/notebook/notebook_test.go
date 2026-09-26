package notebook_test

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/unmango/slip/internal/notebook"
)

func TestFind(t *testing.T) {
	tests := map[string]struct {
		markers []string
		from    string
		want    string
		found   bool
	}{
		"at root":       {[]string{"/notes/.zk"}, "/notes", "/notes", true},
		"one level up":  {[]string{"/notes/.zk"}, "/notes/sub", "/notes", true},
		"deeply nested": {[]string{"/notes/.zk"}, "/notes/a/b/c", "/notes", true},
		"none":          {nil, "/notes/a/b", "", false},
		"nearest wins":  {[]string{"/notes/.zk", "/notes/inner/.zk"}, "/notes/inner/a", "/notes/inner", true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			for _, m := range tt.markers {
				if err := fsys.MkdirAll(m, 0o755); err != nil {
					t.Fatalf("seeding %s: %v", m, err)
				}
			}

			if err := fsys.MkdirAll(tt.from, 0o755); err != nil {
				t.Fatalf("seeding %s: %v", tt.from, err)
			}

			got, found := notebook.Find(fsys, tt.from)
			if found != tt.found {
				t.Fatalf("Find() found = %v, want %v", found, tt.found)
			}

			if got != tt.want {
				t.Errorf("Find() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A file named .zk is not a notebook.
func TestFindIgnoresFile(t *testing.T) {
	fsys := afero.NewMemMapFs()
	if err := afero.WriteFile(fsys, "/notes/.zk", []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if _, found := notebook.Find(fsys, "/notes"); found {
		t.Error("Find() found a notebook, want none")
	}
}

func TestDirPrefersEnv(t *testing.T) {
	fsys := afero.NewMemMapFs()
	if err := fsys.MkdirAll("/notes/.zk", 0o755); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	t.Setenv(notebook.EnvDir, "/elsewhere")

	got, found := notebook.Dir(fsys, "/notes")
	if !found {
		t.Fatal("Dir() found nothing, want the env override")
	}

	if want := "/elsewhere"; got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestDirFallsBackToWalk(t *testing.T) {
	fsys := afero.NewMemMapFs()
	if err := fsys.MkdirAll("/notes/.zk", 0o755); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	t.Setenv(notebook.EnvDir, "")

	got, found := notebook.Dir(fsys, "/notes/sub")
	if !found {
		t.Fatal("Dir() found nothing, want the walked-up notebook")
	}

	if want := "/notes"; got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}
