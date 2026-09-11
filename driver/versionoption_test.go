// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// #1713: the front end refused `--version` as an unknown option for every
// dialect, where three of the four shells in the panel answer it. The option
// is the dialect's — the spelling, the text, the stream and the status — and
// the front end reads all four off the vector.
//
// Named for the axis rather than for a shell: what is under test is that a
// dialect naming a version option is answered from it, and a dialect naming
// none is refused as before. Which dialect does which is dialect/'s to say.
func versionShell(v interp.VersionOption) driver.Shell {
	sem := interp.PosixSemantics()
	sem.VersionOption = v
	return driver.Shell{
		Name:      "testsh",
		Dialect:   syntax.Core(),
		Semantics: sem,
	}
}

func TestAVersionOptionIsAnsweredFromTheVector(t *testing.T) {
	for _, c := range []struct {
		name   string
		opt    interp.VersionOption
		argv   []string
		out    string
		errs   string
		status int
	}{
		{
			name:   "on standard output",
			opt:    interp.VersionOption{Spellings: "--version", Text: "testsh 1.2.3"},
			argv:   []string{"testsh", "--version"},
			out:    "testsh 1.2.3\n",
			status: 0,
		},
		{
			// One shell in the panel names its version on standard error and
			// exits a failure for having been asked.
			name:   "on standard error, at a failing status",
			opt:    interp.VersionOption{Spellings: "--version", Text: "  version   testsh", ToStandardError: true, Status: 2},
			argv:   []string{"testsh", "--version"},
			errs:   "  version   testsh\n",
			status: 2,
		},
		{
			// Measured across the panel: the answer ends the invocation, and
			// the command string after it never runs.
			name:   "before anything the rest of the vector would have run",
			opt:    interp.VersionOption{Spellings: "--version", Text: "testsh 1.2.3"},
			argv:   []string{"testsh", "--version", "-c", "echo ran"},
			out:    "testsh 1.2.3\n",
			status: 0,
		},
		{
			// And it is read wherever it stands, which is what zsh and ksh93
			// both do.
			name:   "after another option word",
			opt:    interp.VersionOption{Spellings: "--version", Text: "testsh 1.2.3"},
			argv:   []string{"testsh", "-x", "--version", "-c", "echo ran"},
			out:    "testsh 1.2.3\n",
			status: 0,
		},
		{
			// A word after the options have ended is an operand, not this:
			// `-c cmd --version` runs the command in all three.
			name:   "not once the options have ended",
			opt:    interp.VersionOption{Spellings: "--version", Text: "testsh 1.2.3"},
			argv:   []string{"testsh", "-c", "echo ran", "--version"},
			out:    "ran\n",
			status: 0,
		},
		{
			// The zero value is a shell with no such option, which is what
			// the panel's fourth member is: the word is refused the way any
			// unknown long option is.
			name:   "refused where the dialect names none",
			opt:    interp.VersionOption{},
			argv:   []string{"testsh", "--version"},
			errs:   "testsh: unknown option \"--version\"\n",
			status: 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs strings.Builder
			sh := versionShell(c.opt)
			sh.Stdout, sh.Stderr = &out, &errs
			if got := driver.MainArgs(sh, c.argv); got != c.status {
				t.Errorf("status %d, want %d (out %q, err %q)", got, c.status, out.String(), errs.String())
			}
			if out.String() != c.out {
				t.Errorf("stdout %q, want %q", out.String(), c.out)
			}
			if errs.String() != c.errs {
				t.Errorf("stderr %q, want %q", errs.String(), c.errs)
			}
		})
	}
}
