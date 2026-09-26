// Command slip captures zettelkasten notes.
//
// Anything slip does not implement is passed through to zk, so a single binary
// covers capture and everything zk already does well.
package main

import (
	"os"

	ucli "github.com/unmango/go/cli"
	"github.com/unmango/slip/internal/cli"
	"github.com/unmango/slip/internal/passthru"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	cli.Version = version

	// Dispatch before cobra sees the arguments: an unowned command must reach
	// zk untouched, rather than being rejected as unknown.
	args := os.Args[1:]
	if !passthru.Owns(args) {
		ucli.Fail(passthru.Exec(args))
	}

	if err := cli.New().Execute(); err != nil {
		ucli.Fail(err)
	}
}
