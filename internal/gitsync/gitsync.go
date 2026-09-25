// Package gitsync publishes captured notes to a git remote.
//
// The model is deliberately narrow, because a zettelkasten is append-only: a
// capture writes a new file and nothing ever edits or deletes one. That makes
// every sync a fast-forward, so this package never merges and never rebases,
// which matters because go-git implements neither.
//
// A sync resets the worktree to the remote and replays the pending notes on
// top. Nothing is committed until the moment it is pushed, and the pending
// notes are held in memory across the attempt, so a rejected push loses
// nothing and simply tries again against the newer remote.
package gitsync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/unmango/slip/internal/note"
	"github.com/go-git/go-billy/v5"
	billyutil "github.com/go-git/go-billy/v5/util"
	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// ext is the file extension zk indexes by default, and the one store writes.
const ext = ".md"

// remoteName is fixed. A capture client has one place to publish to, and
// naming it anything but origin would only surprise whoever clones the
// notebook on a real machine.
const remoteName = "origin"

// attempts bounds the fetch-reset-commit-push loop. Each retry means someone
// else pushed in the window, which is rare and never sustained.
const attempts = 3

// noteFile matches a filename store would have written: a zettel id, which is
// a minute-precision timestamp plus the alphabetic collision suffix NextID
// appends. Anything else untracked in the notebook belongs to the person who
// put it there, and syncing has no business committing it.
var noteFile = regexp.MustCompile(`^[0-9]{12}[a-z]*\.md$`)

// Remote is where notes are published.
//
// Auth is a token over HTTPS rather than a key over SSH: go-git needs the
// private key in process memory, which forfeits the hardware-backed keystore
// that is the only good place for a credential on a phone.
type Remote struct {
	URL string
	// Branch is pushed to directly. A capture client opening pull requests
	// against its own notebook would be ceremony.
	Branch string
	// Username is ignored by GitHub, which only reads the token, but GitLab
	// and Bitbucket require a specific value alongside one.
	Username string
	Token    string
}

// Syncer publishes the notes in a notebook directory.
type Syncer struct {
	dir    string
	remote Remote
	author object.Signature
}

// Result reports what a sync did.
type Result struct {
	// Published counts the notes this sync put on the remote.
	Published int
	// Head is the commit the notebook now sits on.
	Head string
}

func New(dir string, remote Remote, authorName, authorEmail string) *Syncer {
	if remote.Branch == "" {
		remote.Branch = "main"
	}

	if remote.Username == "" {
		// Any non-empty username satisfies GitHub, which authenticates on the
		// token alone, and this is the value it documents.
		remote.Username = "x-access-token"
	}

	return &Syncer{
		dir:    dir,
		remote: remote,
		author: object.Signature{Name: authorName, Email: authorEmail},
	}
}

// pending is a note waiting to reach the remote, held by content rather than
// by path so that a failed push can put it back after the worktree is reset.
type pending struct {
	id      string
	content []byte
}

// Pending reports how many captured notes have not reached the remote. It is
// answered from the local repository alone, so it costs no network and is safe
// to call on every screen.
func (s *Syncer) Pending() (int, error) {
	repo, err := s.open()
	if err != nil {
		return 0, err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return 0, fmt.Errorf("opening the worktree: %w", err)
	}

	remoteTree, err := s.remoteTree(repo)
	if err != nil {
		return 0, err
	}

	notes, err := s.collect(wt.Filesystem, repo, remoteTree)

	return len(notes), err
}

// Sync publishes every pending note.
func (s *Syncer) Sync(ctx context.Context) (Result, error) {
	if s.remote.URL == "" {
		return Result{}, errors.New("no remote configured: set the notebook's git URL first")
	}

	repo, err := s.open()
	if err != nil {
		return Result{}, err
	}

	if err := s.configureRemote(repo); err != nil {
		return Result{}, err
	}

	var last error

	for i := range attempts {
		result, err := s.attempt(ctx, repo)
		if err == nil {
			return result, nil
		}

		// Anything other than losing the race is not going to resolve itself
		// by trying the same thing again.
		if !errors.Is(err, git.ErrNonFastForwardUpdate) {
			return Result{}, err
		}

		last = fmt.Errorf("the remote moved under %d push attempts: %w", i+1, err)
	}

	return Result{}, last
}

// attempt is one pass of the loop: take the pending notes, rewind to the
// remote, replay them, push.
func (s *Syncer) attempt(ctx context.Context, repo *git.Repository) (Result, error) {
	if err := s.fetch(ctx, repo); err != nil {
		return Result{}, err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return Result{}, fmt.Errorf("opening the worktree: %w", err)
	}

	remoteRef, err := s.remoteRef(repo)
	if err != nil {
		return Result{}, err
	}

	remoteTree, err := s.treeOf(repo, remoteRef)
	if err != nil {
		return Result{}, err
	}

	notes, err := s.collect(wt.Filesystem, repo, remoteTree)
	if err != nil {
		return Result{}, err
	}

	// An absent remote branch means the remote is empty, so there is nothing
	// to rewind to and no push that can be rejected. Local history is already
	// the only history.
	if !remoteRef.IsZero() {
		if err := s.rewind(repo, wt, remoteRef); err != nil {
			return Result{}, err
		}
	}

	for _, n := range notes {
		if err := s.commit(wt, remoteTree, n); err != nil {
			return Result{}, err
		}
	}

	if err := s.push(ctx, repo); err != nil {
		return Result{}, err
	}

	head, err := repo.Head()
	if err != nil {
		return Result{}, fmt.Errorf("reading HEAD: %w", err)
	}

	return Result{Published: len(notes), Head: head.Hash().String()}, nil
}

// rewind puts the branch on the remote tip and the worktree with it.
//
// The branch reference is written before the reset rather than left to it,
// because the first sync of a fresh notebook resets an unborn HEAD, and there
// is no reference there for go-git to move.
func (s *Syncer) rewind(repo *git.Repository, wt *git.Worktree, remoteRef plumbing.Hash) error {
	branch := plumbing.NewBranchReferenceName(s.remote.Branch)

	// HEAD follows the configured branch unconditionally. The notebook is this
	// client's to publish, and a checkout left on some other branch would push
	// nothing while reporting success.
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, branch)); err != nil {
		return fmt.Errorf("pointing HEAD at %s: %w", s.remote.Branch, err)
	}

	if err := repo.Storer.SetReference(plumbing.NewHashReference(branch, remoteRef)); err != nil {
		return fmt.Errorf("moving %s to %s: %w", s.remote.Branch, remoteRef, err)
	}

	if err := wt.Reset(&git.ResetOptions{Commit: remoteRef, Mode: git.HardReset}); err != nil {
		return fmt.Errorf("rewinding to %s: %w", s.remote.Branch, err)
	}

	return nil
}

// collect gathers every note that is not yet on the remote, by content.
//
// Two kinds qualify, and the second is what makes a sync interrupted between
// its commit and its push recoverable: a note committed locally last time is
// still absent from the remote, and the reset that is about to happen would
// otherwise take it with it.
func (s *Syncer) collect(fs billy.Filesystem, repo *git.Repository, remoteTree *object.Tree) ([]pending, error) {
	found := map[string][]byte{}

	entries, err := fs.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.dir, err)
	}

	for _, e := range entries {
		if e.IsDir() || !noteFile.MatchString(e.Name()) {
			continue
		}

		b, err := billyutil.ReadFile(fs, e.Name())
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}

		if publishedAlready(remoteTree, e.Name(), b) {
			continue
		}

		found[strings.TrimSuffix(e.Name(), ext)] = b
	}

	// The worktree covers both kinds whenever it is in step with HEAD, which
	// it is in every case but a checkout that lost files. Reading HEAD's tree
	// as well costs one object lookup and closes that gap.
	headTree, err := s.headTree(repo)
	if err != nil {
		return nil, err
	}

	if headTree != nil {
		err = headTree.Files().ForEach(func(f *object.File) error {
			if !noteFile.MatchString(f.Name) {
				return nil
			}

			id := strings.TrimSuffix(f.Name, ext)
			if _, ok := found[id]; ok {
				return nil
			}

			contents, err := f.Contents()
			if err != nil {
				return fmt.Errorf("reading %s from HEAD: %w", f.Name, err)
			}

			if publishedAlready(remoteTree, f.Name, []byte(contents)) {
				return nil
			}

			found[id] = []byte(contents)

			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	out := make([]pending, 0, len(found))
	for id, content := range found {
		out = append(out, pending{id: id, content: content})
	}

	// Oldest first, so the history reads in the order the thoughts happened.
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })

	return out, nil
}

// commit writes one note into the worktree and commits it.
//
// The id is re-resolved against the remote first: another device may have
// captured a note in the same minute, and two notes must never contend for one
// address. Renaming updates the frontmatter alongside the filename, which is
// the invariant store.Create maintains and this must not break.
func (s *Syncer) commit(wt *git.Worktree, remoteTree *object.Tree, n pending) error {
	fs := wt.Filesystem

	id := note.NextID(n.id, func(candidate string) bool {
		name := candidate + ext

		if onRemote(remoteTree, name) {
			return true
		}

		// A note already replayed in this pass holds its name. The note being
		// placed does not collide with itself.
		if candidate == n.id {
			return false
		}

		_, err := fs.Stat(name)

		return err == nil
	})

	content := n.content

	if id != n.id {
		var err error
		if content, err = retitle(n.content, id); err != nil {
			return err
		}

		// Leaving the old path would publish the same thought under two
		// addresses, so it goes, unless the collision was with the remote: the
		// file sitting there now is the other device's note, restored by the
		// rewind, and deleting it is how a sync would lose someone's thought.
		//
		// It is also often already gone, because a note harvested out of local
		// history was removed by that same rewind.
		if !onRemote(remoteTree, n.id+ext) {
			if err := fs.Remove(n.id + ext); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("removing %s: %w", n.id+ext, err)
			}
		}
	}

	name := id + ext

	if err := billyutil.WriteFile(fs, name, content, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", name, err)
	}

	if _, err := wt.Add(name); err != nil {
		return fmt.Errorf("staging %s: %w", name, err)
	}

	parsed, err := note.Unmarshal(content)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", name, err)
	}

	author := s.author
	author.When = parsed.CreateTime
	if author.When.IsZero() {
		author.When = time.Now()
	}

	message := fmt.Sprintf("capture(%s): %s", id, parsed.Title)
	if parsed.Title == "" {
		message = "capture(" + id + ")"
	}

	if _, err := wt.Commit(message, &git.CommitOptions{Author: &author}); err != nil {
		return fmt.Errorf("committing %s: %w", name, err)
	}

	return nil
}

// retitle rewrites a note under a new id, keeping the body verbatim.
func retitle(content []byte, id string) ([]byte, error) {
	n, err := note.Unmarshal(content)
	if err != nil {
		return nil, fmt.Errorf("parsing a note to re-address it: %w", err)
	}

	n.ZettelID = id

	b, err := note.Marshal(n)
	if err != nil {
		return nil, fmt.Errorf("re-addressing a note as %s: %w", id, err)
	}

	return b, nil
}

// publishedAlready reports whether the remote already carries this exact note.
//
// A name match is not enough. Another device capturing in the same minute
// produces a different note at the same address, and reading that as "already
// published" is how this one would be silently dropped.
func publishedAlready(tree *object.Tree, name string, content []byte) bool {
	if tree == nil {
		return false
	}

	entry, err := tree.FindEntry(path.Clean(name))
	if err != nil {
		return false
	}

	return entry.Hash == plumbing.ComputeHash(plumbing.BlobObject, content)
}

// onRemote reports whether the address is occupied on the remote, whoever the
// note there belongs to.
func onRemote(tree *object.Tree, name string) bool {
	if tree == nil {
		return false
	}

	_, err := tree.File(path.Clean(name))

	return err == nil
}

// open returns the notebook's repository, initialising one if the directory is
// not a repository yet. Cloning is never the path: the notebook may already
// hold notes captured before a remote was configured, and fetch-then-reset
// arrives at the same place without demanding an empty directory.
func (s *Syncer) open() (*git.Repository, error) {
	repo, err := git.PlainOpen(s.dir)
	if err == nil {
		return repo, nil
	}

	if !errors.Is(err, git.ErrRepositoryNotExists) {
		return nil, fmt.Errorf("opening %s: %w", s.dir, err)
	}

	repo, err = git.PlainInit(s.dir, false)
	if err != nil {
		return nil, fmt.Errorf("initialising %s: %w", s.dir, err)
	}

	// PlainInit points HEAD at master. The branch that matters is the one the
	// remote uses, and an unborn HEAD is cheap to repoint.
	ref := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(s.remote.Branch))
	if err := repo.Storer.SetReference(ref); err != nil {
		return nil, fmt.Errorf("pointing HEAD at %s: %w", s.remote.Branch, err)
	}

	return repo, nil
}

// configureRemote makes origin agree with the configured URL, which it will
// not after someone moves the notebook to a different host.
func (s *Syncer) configureRemote(repo *git.Repository) error {
	cfg := &gitconfig.RemoteConfig{Name: remoteName, URLs: []string{s.remote.URL}}

	existing, err := repo.Remote(remoteName)
	if errors.Is(err, git.ErrRemoteNotFound) {
		if _, err := repo.CreateRemote(cfg); err != nil {
			return fmt.Errorf("adding the %s remote: %w", remoteName, err)
		}

		return nil
	}

	if err != nil {
		return fmt.Errorf("reading the %s remote: %w", remoteName, err)
	}

	if len(existing.Config().URLs) > 0 && existing.Config().URLs[0] == s.remote.URL {
		return nil
	}

	if err := repo.DeleteRemote(remoteName); err != nil {
		return fmt.Errorf("replacing the %s remote: %w", remoteName, err)
	}

	if _, err := repo.CreateRemote(cfg); err != nil {
		return fmt.Errorf("adding the %s remote: %w", remoteName, err)
	}

	return nil
}

func (s *Syncer) auth() transport.AuthMethod {
	if s.remote.Token == "" {
		return nil
	}

	return &githttp.BasicAuth{Username: s.remote.Username, Password: s.remote.Token}
}

func (s *Syncer) fetch(ctx context.Context, repo *git.Repository) error {
	err := repo.FetchContext(ctx, &git.FetchOptions{
		RemoteName: remoteName,
		Auth:       s.auth(),
		RefSpecs: []gitconfig.RefSpec{
			gitconfig.RefSpec("+refs/heads/*:refs/remotes/" + remoteName + "/*"),
		},
		Force: true,
	})

	// An empty remote and an unchanged one are both successful fetches that
	// happen to have nothing to say.
	if err == nil ||
		errors.Is(err, git.NoErrAlreadyUpToDate) ||
		errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return nil
	}

	return fmt.Errorf("fetching %s: %w", s.remote.URL, err)
}

func (s *Syncer) push(ctx context.Context, repo *git.Repository) error {
	branch := plumbing.NewBranchReferenceName(s.remote.Branch)
	spec := gitconfig.RefSpec(fmt.Sprintf("%s:%s", branch, branch))

	err := repo.PushContext(ctx, &git.PushOptions{
		RemoteName: remoteName,
		Auth:       s.auth(),
		RefSpecs:   []gitconfig.RefSpec{spec},
	})
	if err == nil || errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}

	// Wrapped rather than replaced, so Sync can still recognise the one error
	// worth retrying.
	return fmt.Errorf("pushing to %s: %w", s.remote.Branch, err)
}

// remoteRef is the fetched tip of the tracked branch, or the zero hash when the
// remote has no such branch yet.
func (s *Syncer) remoteRef(repo *git.Repository) (plumbing.Hash, error) {
	ref, err := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, s.remote.Branch), true)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return plumbing.ZeroHash, nil
	}

	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("resolving %s/%s: %w", remoteName, s.remote.Branch, err)
	}

	return ref.Hash(), nil
}

func (s *Syncer) remoteTree(repo *git.Repository) (*object.Tree, error) {
	hash, err := s.remoteRef(repo)
	if err != nil {
		return nil, err
	}

	return s.treeOf(repo, hash)
}

func (s *Syncer) headTree(repo *git.Repository) (*object.Tree, error) {
	head, err := repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("reading HEAD: %w", err)
	}

	return s.treeOf(repo, head.Hash())
}

func (s *Syncer) treeOf(repo *git.Repository, hash plumbing.Hash) (*object.Tree, error) {
	if hash.IsZero() {
		return nil, nil
	}

	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("reading commit %s: %w", hash, err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("reading the tree of %s: %w", hash, err)
	}

	return tree, nil
}
