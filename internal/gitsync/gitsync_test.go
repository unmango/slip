package gitsync_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unmango/slip/internal/gitsync"
	"github.com/unmango/slip/internal/note"
	"github.com/unmango/slip/internal/store"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/afero"
)

var now = time.Date(2026, 9, 8, 14, 12, 33, 0, time.UTC)

// newRemote is an empty bare repository standing in for the notebook's origin.
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

// device is a notebook directory wired to a remote, as one phone would be.
type device struct {
	t      *testing.T
	dir    string
	syncer *gitsync.Syncer
}

func newDevice(t *testing.T, remote string) *device {
	t.Helper()

	dir := t.TempDir()

	return &device{
		t:   t,
		dir: dir,
		syncer: gitsync.New(dir, gitsync.Remote{URL: remote, Branch: "main"},
			"Test Device", "device@example.com"),
	}
}

// capture writes a note the way the capture screen would, at a fixed time so
// the id is predictable.
func (d *device) capture(at time.Time, body string) string {
	d.t.Helper()

	path, err := store.New(afero.NewOsFs(), d.dir).Create(note.New(at, body))
	if err != nil {
		d.t.Fatalf("capturing: %v", err)
	}

	return path
}

func (d *device) sync() {
	d.t.Helper()

	if _, err := d.syncer.Sync(context.Background()); err != nil {
		d.t.Fatalf("Sync() error = %v", err)
	}
}

func (d *device) pending() int {
	d.t.Helper()

	n, err := d.syncer.Pending()
	if err != nil {
		d.t.Fatalf("Pending() error = %v", err)
	}

	return n
}

// published is every note on the remote, by id, with its body.
func published(t *testing.T, remote string) map[string]string {
	t.Helper()

	dir := t.TempDir()

	repo, err := git.PlainClone(dir, false, &git.CloneOptions{URL: remote})
	if err != nil {
		t.Fatalf("cloning the remote: %v", err)
	}

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("reading the remote HEAD: %v", err)
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("reading the remote tip: %v", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		t.Fatalf("reading the remote tree: %v", err)
	}

	out := map[string]string{}

	err = tree.Files().ForEach(func(f *object.File) error {
		contents, err := f.Contents()
		if err != nil {
			return err
		}

		n, err := note.Unmarshal([]byte(contents))
		if err != nil {
			return err
		}

		if want := f.Name; want != n.ZettelID+".md" {
			t.Errorf("note %s carries zettel_id %q", f.Name, n.ZettelID)
		}

		out[n.ZettelID] = n.Body

		return nil
	})
	if err != nil {
		t.Fatalf("walking the remote tree: %v", err)
	}

	return out
}

func TestSyncPublishesACapture(t *testing.T) {
	remote := newRemote(t)
	d := newDevice(t, remote)

	d.capture(now, "a thought worth keeping\n")

	if got := d.pending(); got != 1 {
		t.Fatalf("Pending() = %d, want 1", got)
	}

	d.sync()

	got := published(t, remote)
	if want := "a thought worth keeping\n"; got["202609081412"] != want {
		t.Errorf("published body = %q, want %q", got["202609081412"], want)
	}

	if len(got) != 1 {
		t.Errorf("published %d notes, want 1", len(got))
	}

	if n := d.pending(); n != 0 {
		t.Errorf("Pending() after Sync() = %d, want 0", n)
	}
}

func TestSyncIsIdempotent(t *testing.T) {
	remote := newRemote(t)
	d := newDevice(t, remote)

	d.capture(now, "once\n")
	d.sync()
	d.sync()

	if got := published(t, remote); len(got) != 1 {
		t.Errorf("published %d notes over two syncs, want 1", len(got))
	}
}

func TestSyncPublishesInCaptureOrder(t *testing.T) {
	remote := newRemote(t)
	d := newDevice(t, remote)

	d.capture(now.Add(-time.Hour), "earlier\n")
	d.capture(now, "later\n")
	d.sync()

	if got := published(t, remote); len(got) != 2 {
		t.Fatalf("published %d notes, want 2", len(got))
	}
}

// Two devices capturing in the same minute is the collision the whole
// re-addressing path exists for. Neither thought may be lost or overwritten.
func TestSyncReAddressesACollidingID(t *testing.T) {
	remote := newRemote(t)

	other := newDevice(t, remote)
	other.capture(now, "from the other device\n")
	other.sync()

	d := newDevice(t, remote)
	d.capture(now, "from this device\n")
	d.sync()

	got := published(t, remote)
	if len(got) != 2 {
		t.Fatalf("published %d notes, want 2: %v", len(got), got)
	}

	if want := "from the other device\n"; got["202609081412"] != want {
		t.Errorf("202609081412 = %q, want %q", got["202609081412"], want)
	}

	if want := "from this device\n"; got["202609081412a"] != want {
		t.Errorf("202609081412a = %q, want %q", got["202609081412a"], want)
	}

	// The re-addressed note must not also survive under its old name.
	if _, err := os.Stat(filepath.Join(d.dir, "202609081412a.md")); err != nil {
		t.Errorf("the re-addressed note is missing locally: %v", err)
	}
}

// A sync that commits and then loses the network leaves notes in local history
// and not on the remote. The next sync rewinds past those commits, so it has to
// carry the notes across.
func TestSyncRecoversNotesCommittedButNotPushed(t *testing.T) {
	remote := newRemote(t)

	d := newDevice(t, remote)
	d.capture(now, "stranded\n")

	// Commit locally without pushing, which is exactly the state an
	// interrupted sync leaves behind.
	commitLocally(t, d.dir, "202609081412.md")

	if got := d.pending(); got != 1 {
		t.Fatalf("Pending() = %d, want 1: a committed note is still unpublished", got)
	}

	// Meanwhile another device publishes, so the rewind is a real one.
	other := newDevice(t, remote)
	other.capture(now.Add(time.Minute), "from the other device\n")
	other.sync()

	d.sync()

	got := published(t, remote)
	if want := "stranded\n"; got["202609081412"] != want {
		t.Errorf("202609081412 = %q, want %q", got["202609081412"], want)
	}

	if want := "from the other device\n"; got["202609081413"] != want {
		t.Errorf("202609081413 = %q, want %q", got["202609081413"], want)
	}
}

// A push rejected because the remote moved is retried against the new tip
// rather than failing the sync.
func TestSyncRetriesAfterTheRemoteMoves(t *testing.T) {
	remote := newRemote(t)

	seed := newDevice(t, remote)
	seed.capture(now.Add(-time.Hour), "seed\n")
	seed.sync()

	d := newDevice(t, remote)
	d.sync() // catch up, so d has a stale view to go stale from

	other := newDevice(t, remote)
	other.capture(now.Add(time.Minute), "raced in\n")
	other.sync()

	d.capture(now, "mine\n")
	d.sync()

	got := published(t, remote)
	if len(got) != 3 {
		t.Fatalf("published %d notes, want 3: %v", len(got), got)
	}
}

func TestSyncWithoutARemoteIsAnError(t *testing.T) {
	d := gitsync.New(t.TempDir(), gitsync.Remote{}, "Test", "test@example.com")

	if _, err := d.Sync(context.Background()); err == nil {
		t.Fatal("Sync() with no remote returned no error")
	}
}

// Anything that is not a note is the notebook owner's, and syncing it would be
// the tool taking a liberty with someone else's files.
func TestSyncLeavesForeignFilesAlone(t *testing.T) {
	remote := newRemote(t)
	d := newDevice(t, remote)

	if err := os.WriteFile(filepath.Join(d.dir, "README.md"), []byte("# notebook\n"), 0o644); err != nil {
		t.Fatalf("writing README.md: %v", err)
	}

	d.capture(now, "a thought\n")

	if got := d.pending(); got != 1 {
		t.Errorf("Pending() = %d, want 1", got)
	}

	d.sync()

	if _, ok := published(t, remote)["README"]; ok {
		t.Error("README.md was published")
	}
}

func TestPendingOnAFreshNotebook(t *testing.T) {
	d := newDevice(t, newRemote(t))

	if got := d.pending(); got != 0 {
		t.Errorf("Pending() = %d, want 0", got)
	}
}

// commitLocally stages and commits one file, without pushing.
func commitLocally(t *testing.T, dir, name string) {
	t.Helper()

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("initialising %s: %v", dir, err)
	}

	head := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))
	if err := repo.Storer.SetReference(head); err != nil {
		t.Fatalf("pointing HEAD at main: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("opening the worktree: %v", err)
	}

	if _, err := wt.Add(name); err != nil {
		t.Fatalf("staging %s: %v", name, err)
	}

	sig := &object.Signature{Name: "Test Device", Email: "device@example.com", When: now}
	if _, err := wt.Commit("capture: stranded", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("committing %s: %v", name, err)
	}
}
