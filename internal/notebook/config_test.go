package notebook_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/unmango/slip/internal/notebook"
	"github.com/spf13/afero"
)

func TestInspectConfig(t *testing.T) {
	tests := map[string]struct {
		content string
		exists  bool
		want    notebook.ConfigState
	}{
		"no file":      {"", false, notebook.ConfigMissing},
		"empty file":   {"", true, notebook.ConfigMissing},
		"unrelated":    {"[note]\nfilename = \"{{id}}\"\n", true, notebook.ConfigMissing},
		"already set":  {notebook.Stanza, true, notebook.ConfigPresent},
		"other table":  {"[format.markdown.frontmatter]\ncreation-date-key = \"date\"\n", true, notebook.ConfigConflict},
		"bare table":   {"[format.markdown.frontmatter]\n", true, notebook.ConfigConflict},
		"among others": {"[note]\nextension = \"md\"\n\n" + notebook.Stanza, true, notebook.ConfigPresent},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			if tt.exists {
				path := filepath.Join("/notes", notebook.ConfigPath)
				if err := afero.WriteFile(fsys, path, []byte(tt.content), 0o644); err != nil {
					t.Fatalf("seeding: %v", err)
				}
			}

			got, err := notebook.InspectConfig(fsys, "/notes")
			if err != nil {
				t.Fatalf("InspectConfig() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("InspectConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWriteConfigCreates(t *testing.T) {
	fsys := afero.NewMemMapFs()

	if err := notebook.WriteConfig(fsys, "/notes"); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	b, err := afero.ReadFile(fsys, filepath.Join("/notes", notebook.ConfigPath))
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}

	if string(b) != notebook.Stanza {
		t.Errorf("wrote %q, want %q", b, notebook.Stanza)
	}
}

// An existing config must survive, with the stanza appended and separated.
func TestWriteConfigAppends(t *testing.T) {
	fsys := afero.NewMemMapFs()
	path := filepath.Join("/notes", notebook.ConfigPath)

	existing := "[note]\nfilename = \"{{id}}\"\n"
	if err := afero.WriteFile(fsys, path, []byte(existing), 0o644); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if err := notebook.WriteConfig(fsys, "/notes"); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	b, err := afero.ReadFile(fsys, path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}

	got := string(b)
	if !strings.HasPrefix(got, existing) {
		t.Errorf("existing config was lost:\n%s", got)
	}

	if !strings.Contains(got, "creation-date-key") {
		t.Errorf("stanza was not appended:\n%s", got)
	}

	// Appending must declare the table once. Repeated invocation is covered by
	// TestWriteConfigIsIdempotent.
	if strings.Count(got, "[format.markdown.frontmatter]") != 1 {
		t.Errorf("table appears more than once:\n%s", got)
	}
}

// A file with no trailing newline must not have the stanza run onto its last
// line, which would produce invalid TOML.
func TestWriteConfigAddsSeparator(t *testing.T) {
	fsys := afero.NewMemMapFs()
	path := filepath.Join("/notes", notebook.ConfigPath)

	if err := afero.WriteFile(fsys, path, []byte("[note]\nextension = \"md\""), 0o644); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if err := notebook.WriteConfig(fsys, "/notes"); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	b, err := afero.ReadFile(fsys, path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}

	if !strings.Contains(string(b), "\"md\"\n\n[format.markdown.frontmatter]") {
		t.Errorf("stanza is not separated from the previous line:\n%s", b)
	}
}

// zk's own starter config documents this table in comments. Reading those as a
// real declaration would report a conflict on every freshly created notebook.
func TestInspectConfigIgnoresComments(t *testing.T) {
	const zkDefault = `# Filename and extension
[note]
extension = "md"

# MARKDOWN SETTINGS
#[format.markdown.frontmatter]

# Define custom keys and properties of the frontmatter block
#creation-date-key = "created" # default is "date"
`

	fsys := afero.NewMemMapFs()
	path := filepath.Join("/notes", notebook.ConfigPath)
	if err := afero.WriteFile(fsys, path, []byte(zkDefault), 0o644); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	got, err := notebook.InspectConfig(fsys, "/notes")
	if err != nil {
		t.Fatalf("InspectConfig() error = %v", err)
	}

	if got != notebook.ConfigMissing {
		t.Errorf("InspectConfig() = %v, want ConfigMissing", got)
	}
}

// A trailing comment after a real setting must not hide it.
func TestInspectConfigTrailingComment(t *testing.T) {
	fsys := afero.NewMemMapFs()
	path := filepath.Join("/notes", notebook.ConfigPath)
	content := "[format.markdown.frontmatter]\ncreation-date-key = \"create_time\" # set by slip\n"
	if err := afero.WriteFile(fsys, path, []byte(content), 0o644); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	got, err := notebook.InspectConfig(fsys, "/notes")
	if err != nil {
		t.Fatalf("InspectConfig() error = %v", err)
	}

	if got != notebook.ConfigPresent {
		t.Errorf("InspectConfig() = %v, want ConfigPresent", got)
	}
}

// Appending unconditionally would declare the table twice, which is invalid
// TOML. Writing an already-configured notebook must change nothing.
func TestWriteConfigIsIdempotent(t *testing.T) {
	fsys := afero.NewMemMapFs()
	path := filepath.Join("/notes", notebook.ConfigPath)

	if err := notebook.WriteConfig(fsys, "/notes"); err != nil {
		t.Fatalf("first WriteConfig() error = %v", err)
	}

	first, err := afero.ReadFile(fsys, path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}

	if err := notebook.WriteConfig(fsys, "/notes"); err != nil {
		t.Fatalf("second WriteConfig() error = %v", err)
	}

	second, err := afero.ReadFile(fsys, path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}

	if string(first) != string(second) {
		t.Errorf("second write changed the file:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}

	if n := strings.Count(string(second), "[format.markdown.frontmatter]"); n != 1 {
		t.Errorf("table appears %d times after two writes, want 1:\n%s", n, second)
	}
}
