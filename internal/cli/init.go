package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/unmango/slip/internal/notebook"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

func newInit() *cobra.Command {
	var printOnly bool

	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Teach a zk notebook about slip's frontmatter",
		Long: "Adds the one setting zk needs to read a slip note's creation time.\n\n" +
			"Everything else already interoperates: zk keeps unrecognised frontmatter\n" +
			"keys verbatim, so zettel_id, note_type, and format survive untouched.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, args, printOnly)
		},
	}

	cmd.Flags().BoolVar(&printOnly, "print", false, "print the configuration instead of writing it")

	return cmd
}

func runInit(cmd *cobra.Command, args []string, printOnly bool) error {
	out := cmd.OutOrStdout()

	if printOnly {
		fmt.Fprint(out, notebook.Stanza)
		return nil
	}

	fsys := afero.NewOsFs()

	root, err := initRoot(fsys, args)
	if err != nil {
		return err
	}

	state, err := notebook.InspectConfig(fsys, root)
	if err != nil {
		return err
	}

	path := filepath.Join(root, notebook.ConfigPath)

	switch state {
	case notebook.ConfigPresent:
		fmt.Fprintf(out, "%s already points zk at slip's frontmatter\n", path)
		return nil

	case notebook.ConfigConflict:
		// Appending would declare the table twice, which is invalid TOML.
		// Corrupting a config file is worse than asking for one edit.
		return fmt.Errorf(
			"%s already has a [format.markdown.frontmatter] table.\n"+
				"Add this line to it by hand:\n\n    creation-date-key = \"create_time\"",
			path,
		)

	default:
		if err := notebook.WriteConfig(fsys, root); err != nil {
			return err
		}

		fmt.Fprintf(out, "wrote %s\n", path)

		return nil
	}
}

// initRoot resolves which notebook to configure.
func initRoot(fsys afero.Fs, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolving the working directory: %w", err)
	}

	root, ok := notebook.Dir(fsys, cwd)
	if !ok {
		return "", fmt.Errorf("no zk notebook found from %s: run `zk init` first, or pass a directory", cwd)
	}

	return root, nil
}
