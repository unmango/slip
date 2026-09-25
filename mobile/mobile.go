// Package mobile is the binding surface for the Android and iOS capture apps.
//
// Everything here exists to satisfy gomobile, which binds only a restricted
// set of types: strings, numbers, errors, and structs of those. No behaviour
// lives in this package. Capture is note plus store, publishing is gitsync,
// and both are testable without a phone attached.
//
// Build the Android archive with:
//
//	gomobile bind -target=android ./mobile
package mobile

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/unmango/slip/internal/gitsync"
	"github.com/unmango/slip/internal/note"
	"github.com/unmango/slip/internal/store"
	"github.com/spf13/afero"
)

// syncTimeout bounds a publish. A phone that has wandered onto a captive
// portal will otherwise hold the network call open until the OS kills it.
const syncTimeout = 2 * time.Minute

// Config is the client's settings. It is a struct rather than seven arguments
// because gomobile renders those as a seven-argument Kotlin constructor.
//
// Token is a personal access token over HTTPS. It belongs in EncryptedShared-
// Preferences or the iOS keychain, and is passed in per session rather than
// stored here.
type Config struct {
	Dir         string
	RemoteURL   string
	Branch      string
	Username    string
	Token       string
	AuthorName  string
	AuthorEmail string
}

// NewConfig returns an empty Config. gomobile binds a constructor; it does not
// bind a struct literal.
func NewConfig() *Config {
	return &Config{}
}

// Client is one notebook on one device.
type Client struct {
	dir    string
	syncer *gitsync.Syncer
}

// syncMu keeps two syncs off one worktree, and errMu guards lastErr.
//
// Both are package level rather than fields, because a Client is built fresh
// wherever one is needed: Android constructs one per activity and another
// inside the worker that publishes in the background, and they all point at the
// same directory. A lock that lives on the instance would guard nothing, and a
// failure recorded on the worker's client would be unreadable from the screen,
// which is the only place anyone would see it.
var (
	syncMu sync.Mutex

	errMu   sync.Mutex
	lastErr string
)

func NewClient(cfg *Config) *Client {
	return &Client{
		dir: cfg.Dir,
		syncer: gitsync.New(cfg.Dir, gitsync.Remote{
			URL:      cfg.RemoteURL,
			Branch:   cfg.Branch,
			Username: cfg.Username,
			Token:    cfg.Token,
		}, cfg.AuthorName, cfg.AuthorEmail),
	}
}

// Capture writes a note and returns the path it wrote. It does not touch the
// network: a capture that can fail because of signal is not a capture tool.
func (c *Client) Capture(body string) (string, error) {
	if strings.TrimSpace(body) == "" {
		return "", errors.New("nothing to capture")
	}

	return store.New(afero.NewOsFs(), c.dir).Create(note.New(time.Now(), body))
}

// Pending is how many captured notes have not reached the remote. It answers
// from local state alone, so the capture screen can show it without waiting.
func (c *Client) Pending() int {
	n, err := c.syncer.Pending()
	if err != nil {
		setErr(err)
		return 0
	}

	return n
}

// Sync publishes everything pending.
func (c *Client) Sync() error {
	syncMu.Lock()
	defer syncMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()

	if _, err := c.syncer.Sync(ctx); err != nil {
		setErr(err)
		return err
	}

	setErr(nil)

	return nil
}

// LastError is the most recent failure, for the status line. Pending returns a
// bare count because a number is what the screen wants, so this is where the
// reason it might be wrong goes.
//
// It reports failures from every client in the process, which is what makes a
// sync that failed in the background readable from the capture screen.
func (c *Client) LastError() string {
	errMu.Lock()
	defer errMu.Unlock()

	return lastErr
}

func setErr(err error) {
	errMu.Lock()
	defer errMu.Unlock()

	if err == nil {
		lastErr = ""
		return
	}

	lastErr = err.Error()
}
