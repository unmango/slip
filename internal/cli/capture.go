package cli

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	notev1 "github.com/UnstoppableMango/zettelkasten/gen/unmango/zettelkasten/note/v1alpha1"
	"github.com/UnstoppableMango/zettelkasten/internal/config"
	"github.com/UnstoppableMango/zettelkasten/internal/note"
	"github.com/UnstoppableMango/zettelkasten/internal/store"
	"github.com/UnstoppableMango/zettelkasten/internal/tui"
	"github.com/UnstoppableMango/zettelkasten/internal/zk"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

type captureOptions struct {
	dir       string
	title     string
	noteType  string
	tags      []string
	sourceURI string
}

func newCapture() *cobra.Command {
	var opts captureOptions

	cmd := &cobra.Command{
		Use:   "capture [text]",
		Short: "Capture a thought",
		Long: "With arguments, captures them directly. With input piped in, captures that.\n" +
			"Otherwise opens an editor. Prints the path of the note it wrote.",
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCapture(cmd, args, &opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.dir, "dir", "", "directory to write the note to")
	f.StringVar(&opts.title, "title", "", "override the derived title")
	f.StringVar(&opts.noteType, "type", "fleeting", "note type (fleeting, literature, permanent, structure)")
	f.StringSliceVar(&opts.tags, "tag", nil, "tag to attach (repeatable)")
	f.StringVar(&opts.sourceURI, "source", "", "URI this note came from")

	return cmd
}

func runCapture(cmd *cobra.Command, args []string, opts *captureOptions) error {
	noteType, err := parseNoteType(opts.noteType)
	if err != nil {
		return err
	}

	fsys := afero.NewOsFs()

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolving the working directory: %w", err)
	}

	cfg, err := config.Resolve(fsys, cwd, opts.dir)
	if err != nil {
		return err
	}

	body, ok, err := readBody(cmd, args, cfg, noteType)
	if err != nil {
		return err
	}

	// Discarded, or nothing worth keeping. Not an error: deciding a thought is
	// not worth saving is a normal outcome.
	if !ok || strings.TrimSpace(body) == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "nothing captured")
		return nil
	}

	n := note.New(time.Now(), body)
	n.Type = noteType
	n.Tags = opts.tags
	n.SourceURI = opts.sourceURI

	if opts.title != "" {
		n.Title = opts.title
	}

	path, err := store.New(fsys, cfg.Dir).Create(n)
	if err != nil {
		return err
	}

	// Only the path, so `$EDITOR "$(slip)"` composes.
	fmt.Fprintln(cmd.OutOrStdout(), path)

	return nil
}

// readBody collects the note text from whichever source the invocation implies.
// The bool reports whether the user went through with it.
func readBody(cmd *cobra.Command, args []string, cfg config.Config, noteType notev1.NoteType) (string, bool, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), true, nil
	}

	piped, err := stdinIsPiped()
	if err != nil {
		return "", false, err
	}

	if piped {
		b, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", false, fmt.Errorf("reading stdin: %w", err)
		}

		return string(b), true, nil
	}

	return runTUI(cfg, noteType)
}

func runTUI(cfg config.Config, noteType notev1.NoteType) (string, bool, error) {
	id := note.ZettelID(time.Now())
	path := store.New(afero.NewOsFs(), cfg.Dir).Path(id)

	var links tui.LinkSource
	if cfg.Notebook {
		links = notebookLinks(cfg.Dir)
	}

	model, err := tea.NewProgram(
		tui.New(id, shortNoteType(noteType), path, links),
	).Run()
	if err != nil {
		return "", false, fmt.Errorf("running the capture screen: %w", err)
	}

	result := model.(tui.Model).Result()

	return result.Body, result.Saved, nil
}

// notebookLinks lists dir's notes through zk, most recently modified first,
// since the note being linked to is usually one written lately.
func notebookLinks(dir string) tui.LinkSource {
	return func() ([]tui.Link, error) {
		notes, err := zk.List(context.Background(), dir)
		if err != nil {
			return nil, err
		}

		slices.SortStableFunc(notes, func(a, b zk.Note) int {
			return b.Modified.Compare(a.Modified)
		})

		links := make([]tui.Link, len(notes))
		for i, n := range notes {
			links[i] = tui.Link{
				ID:    cmp.Or(n.ZettelID(), n.FilenameStem),
				Title: n.Title,
				Text:  n.Link,
			}
		}

		return links, nil
	}
}

// stdinIsPiped reports whether stdin is something other than a terminal, which
// is what makes `pbpaste | slip` and `slip < file.md` work.
func stdinIsPiped() (bool, error) {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false, fmt.Errorf("inspecting stdin: %w", err)
	}

	return info.Mode()&os.ModeCharDevice == 0, nil
}

// parseNoteType accepts the short names a person would type rather than the
// full proto enum names the file on disk carries.
func parseNoteType(s string) (notev1.NoteType, error) {
	name := "NOTE_TYPE_" + strings.ToUpper(s)

	v, ok := notev1.NoteType_value[name]
	if !ok || v == int32(notev1.NoteType_NOTE_TYPE_UNSPECIFIED) {
		return 0, fmt.Errorf("unknown note type %q: want fleeting, literature, permanent, or structure", s)
	}

	return notev1.NoteType(v), nil
}

// shortNoteType is the inverse, for display.
func shortNoteType(t notev1.NoteType) string {
	return strings.ToLower(strings.TrimPrefix(t.String(), "NOTE_TYPE_"))
}
