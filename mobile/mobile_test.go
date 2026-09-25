package mobile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unmango/slip/internal/note"
	"github.com/unmango/slip/mobile"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func newRemote(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	repo, err := git.PlainInit(dir, true)
	if err != nil {
		t.Fatalf("initialising the remote: %v", err)
	}

	head := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))
	if err := repo.Storer.SetReference(head); err != nil {
		t.Fatalf("pointing the remote at main: %v", err)
	}

	return dir
}

func newClient(t *testing.T, remote string) (*mobile.Client, string) {
	t.Helper()

	dir := t.TempDir()

	cfg := mobile.NewConfig()
	cfg.Dir = dir
	cfg.RemoteURL = remote
	cfg.Branch = "main"
	cfg.AuthorName = "Test Device"
	cfg.AuthorEmail = "device@example.com"

	return mobile.NewClient(cfg), dir
}

func TestCaptureWritesANote(t *testing.T) {
	c, dir := newClient(t, newRemote(t))

	path, err := c.Capture("a thought worth keeping\n")
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	if got := filepath.Dir(path); got != dir {
		t.Errorf("Capture() wrote to %q, want %q", got, dir)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}

	n, err := note.Unmarshal(b)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if want := "a thought worth keeping\n"; n.Body != want {
		t.Errorf("body = %q, want %q", n.Body, want)
	}
}

// Capture must not need the network. A capture tool that can fail because of
// signal is not one.
func TestCaptureWorksWithAnUnreachableRemote(t *testing.T) {
	c, _ := newClient(t, filepath.Join(t.TempDir(), "nowhere"))

	if _, err := c.Capture("offline\n"); err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
}

func TestCaptureRejectsBlankInput(t *testing.T) {
	c, _ := newClient(t, newRemote(t))

	if _, err := c.Capture("   \n\t"); err == nil {
		t.Error("Capture() of whitespace returned no error")
	}
}

func TestCaptureThenSync(t *testing.T) {
	remote := newRemote(t)
	c, _ := newClient(t, remote)

	if _, err := c.Capture("a thought\n"); err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	if got := c.Pending(); got != 1 {
		t.Fatalf("Pending() = %d, want 1", got)
	}

	if err := c.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if got := c.Pending(); got != 0 {
		t.Errorf("Pending() after Sync() = %d, want 0", got)
	}

	if got := c.LastError(); got != "" {
		t.Errorf("LastError() = %q, want empty", got)
	}
}

func TestSyncRecordsItsFailure(t *testing.T) {
	c, _ := newClient(t, filepath.Join(t.TempDir(), "nowhere"))

	if _, err := c.Capture("a thought\n"); err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	if err := c.Sync(); err == nil {
		t.Fatal("Sync() to a missing remote returned no error")
	}

	if got := c.LastError(); got == "" {
		t.Error("LastError() is empty after a failed Sync()")
	}
}

// The capture screen builds its own client and asks it what went wrong. The
// sync that failed happened on a different client, inside a background worker
// that has since discarded it, so an error recorded per instance would never
// reach the only place anyone would read it.
func TestLastErrorIsVisibleFromAnotherClient(t *testing.T) {
	dir := t.TempDir()

	broken := mobile.NewConfig()
	broken.Dir = dir
	broken.RemoteURL = filepath.Join(t.TempDir(), "nowhere")
	broken.Branch = "main"

	worker := mobile.NewClient(broken)
	if _, err := worker.Capture("a thought\n"); err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	if err := worker.Sync(); err == nil {
		t.Fatal("Sync() to a missing remote returned no error")
	}

	screen := mobile.NewClient(broken)
	if got := screen.LastError(); got == "" {
		t.Error("LastError() on a second client is empty after a failed Sync()")
	}
}

// A failed sync must be recoverable by fixing the remote, not by recapturing.
func TestSyncClearsTheLastErrorOnceItSucceeds(t *testing.T) {
	remote := newRemote(t)
	c, dir := newClient(t, remote)

	broken := mobile.NewConfig()
	broken.Dir = dir
	broken.RemoteURL = filepath.Join(t.TempDir(), "nowhere")
	broken.Branch = "main"

	first := mobile.NewClient(broken)
	if _, err := first.Capture("a thought\n"); err != nil {
		t.Fatalf("Capture() error = %v", err)
	}

	if err := first.Sync(); err == nil {
		t.Fatal("Sync() to a missing remote returned no error")
	}

	if err := c.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if got := c.LastError(); got != "" {
		t.Errorf("LastError() = %q, want empty", got)
	}
}

// gomobile binds only a restricted set of types. Anything it cannot render
// stops the Android build rather than failing a test, so the signatures are
// pinned here instead.
func TestBindableSurface(t *testing.T) {
	var (
		_ func(*mobile.Config) *mobile.Client = mobile.NewClient
		_ func() *mobile.Config               = mobile.NewConfig
		c                                     = mobile.NewClient(mobile.NewConfig())
		_ func(string) (string, error)        = c.Capture
		_ func() int                          = c.Pending
		_ func() error                        = c.Sync
		_ func() string                       = c.LastError
	)

	cfg := mobile.NewConfig()
	if strings.TrimSpace(cfg.Dir) != "" {
		t.Errorf("NewConfig().Dir = %q, want empty", cfg.Dir)
	}
}
