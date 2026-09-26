package passthru_test

import (
	"strings"
	"testing"

	"github.com/unmango/slip/internal/passthru"
)

func TestOwns(t *testing.T) {
	tests := map[string]struct {
		args []string
		want bool
	}{
		"no args is capture":     {nil, true},
		"empty slice is capture": {[]string{}, true},
		"capture":                {[]string{"capture"}, true},
		"capture with text":      {[]string{"capture", "a", "thought"}, true},
		"init":                   {[]string{"init"}, true},
		"help":                   {[]string{"help"}, true},
		"completion":             {[]string{"completion", "zsh"}, true},
		"long flag":              {[]string{"--version"}, true},
		"short flag":             {[]string{"-h"}, true},
		"flag with value":        {[]string{"--dir", "/notes"}, true},

		// zk's surface, which slip deliberately does not shadow.
		"zk list":  {[]string{"list"}, false},
		"zk edit":  {[]string{"edit"}, false},
		"zk new":   {[]string{"new", "--title", "x"}, false},
		"zk index": {[]string{"index"}, false},
		"zk graph": {[]string{"graph"}, false},
		"zk lsp":   {[]string{"lsp"}, false},
		"unknown":  {[]string{"nonsense"}, false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := passthru.Owns(tt.args); got != tt.want {
				t.Errorf("Owns(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

// Without zk on PATH the failure has to name what was missing and what was
// being attempted, rather than reading as an unknown command.
func TestExecReportsMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := passthru.Exec([]string{"list"})
	if err == nil {
		t.Fatal("Exec() = nil error, want an error")
	}

	for _, want := range []string{"list", passthru.Binary, "PATH"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// zk has global options of its own. Claiming every leading dash would reject
// them here rather than letting zk act on them.
func TestOwnsFlags(t *testing.T) {
	tests := map[string]struct {
		args []string
		want bool
	}{
		// slip's own, registered on the root command.
		"--dir":        {[]string{"--dir", "/notes"}, true},
		"--dir=":       {[]string{"--dir=/notes"}, true},
		"--title":      {[]string{"--title", "x"}, true},
		"--type":       {[]string{"--type", "permanent"}, true},
		"--tag":        {[]string{"--tag", "systems"}, true},
		"--source":     {[]string{"--source", "https://example.com"}, true},
		"--help":       {[]string{"--help"}, true},
		"--version":    {[]string{"--version"}, true},
		"-h":           {[]string{"-h"}, true},
		"stdin dash":   {[]string{"-"}, true},
		"end of flags": {[]string{"--"}, true},

		// zk's, which must pass through untouched.
		"--notebook-dir": {[]string{"--notebook-dir", "/notes", "list"}, false},
		"--no-input":     {[]string{"--no-input", "list"}, false},
		"--working-dir":  {[]string{"--working-dir", "/notes"}, false},
		"unknown long":   {[]string{"--nonsense"}, false},
		"unknown short":  {[]string{"-Z"}, false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := passthru.Owns(tt.args); got != tt.want {
				t.Errorf("Owns(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
