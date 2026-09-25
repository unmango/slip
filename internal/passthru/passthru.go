// Package passthru hands invocations slip does not implement to zk.
//
// zk is invoked as a separate process, never linked. It is GPL-3.0 and this is
// not; running a program is not deriving from it.
package passthru

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/unmango/slip/internal/cli"
)

// Binary is the command unowned invocations are handed to.
const Binary = "zk"

// builtins are the commands cobra adds for itself. They are only registered
// once a command runs, so Owns cannot discover them by asking.
var builtins = []string{"help", "completion"}

// Owns reports whether slip handles this argv itself. Everything else belongs
// to zk, which is what makes one binary cover both.
func Owns(args []string) bool {
	// Bare `slip` captures.
	if len(args) == 0 {
		return true
	}

	first := args[0]

	// A bare "-" is the read-stdin convention, and "--" ends slip's own options.
	if first == "-" || first == "--" {
		return true
	}

	if strings.HasPrefix(first, "-") {
		return ownsFlag(first)
	}

	return slices.Contains(builtins, first) || cli.Owned(first)
}

// ownsFlag reports whether arg names an option slip registers. zk has global
// options of its own, such as --notebook-dir, and treating every leading dash
// as slip's would reject them here instead of letting zk act on them.
func ownsFlag(arg string) bool {
	flags := cli.Flags()

	if long, ok := strings.CutPrefix(arg, "--"); ok {
		name, _, _ := strings.Cut(long, "=")
		return flags.Lookup(name) != nil
	}

	short, _, _ := strings.Cut(strings.TrimPrefix(arg, "-"), "=")

	// Shorthands combine, as in -abc, and the whole cluster has to be slip's
	// for the invocation to be.
	for _, r := range short {
		if flags.ShorthandLookup(string(r)) == nil {
			return false
		}
	}

	return short != ""
}

// Exec replaces this process with zk. Replacing rather than wrapping means
// signals, exit codes, and the terminal all belong to zk, with nothing of ours
// left in the pipeline to get them wrong.
func Exec(args []string) error {
	path, err := exec.LookPath(Binary)
	if err != nil {
		return fmt.Errorf(
			"%s is not a slip command, and %s was not found on PATH to pass it to: %w",
			args[0], Binary, err,
		)
	}

	return run(path, append([]string{Binary}, args...))
}
