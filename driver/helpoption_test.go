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

// #3268: the front end refused `--help` as an unknown option in the one
// dialect that answers it. The option is the dialect's — the spelling, the
// version line over the block, the trailer under it, the stream and the status
// — and the block between them is the one Diagnostics already holds, which is
// measured and not assumed.
//
// Named for the axis rather than for a shell, as the version option next door
// is: what is under test is that a dialect naming a help option is answered
// from it, and a dialect naming none is refused as before.
func helpShell(h interp.HelpOption, usage string) driver.Shell {
	sem := interp.PosixSemantics()
	sem.HelpOption = h
	dg := interp.Diagnostics{
		InvocationUsage:         usage,
		InvocationBadLongOption: "%[1]s: invalid option",
	}
	return driver.Shell{
		Name:        "testsh",
		Dialect:     syntax.Core(),
		Semantics:   sem,
		Diagnostics: dg,
	}
}

func TestAHelpOptionIsAnsweredFromTheVector(t *testing.T) {
	const usage = "Usage:\t%[1]s [option] ..."
	full := interp.HelpOption{
		Spellings: "--help",
		Text:      "testsh 1.2.3",
		Trailer:   "Type `%[1]s -c help' for more.",
	}
	for _, c := range []struct {
		name   string
		opt    interp.HelpOption
		usage  string
		argv   []string
		out    string
		errs   string
		status int
	}{
		{
			name:   "the line, the block and the trailer",
			opt:    full,
			usage:  usage,
			argv:   []string{"testsh", "--help"},
			out:    "testsh 1.2.3\nUsage:\ttestsh [option] ...\nType `testsh -c help' for more.\n",
			status: 0,
		},
		{
			// Measured: the answer ends the invocation and the command
			// string after it never runs.
			name:   "before anything the rest of the vector would have run",
			opt:    full,
			usage:  usage,
			argv:   []string{"testsh", "--help", "-c", "echo ran"},
			out:    "testsh 1.2.3\nUsage:\ttestsh [option] ...\nType `testsh -c help' for more.\n",
			status: 0,
		},
		{
			// And a refused word behind it still wins, which is why the
			// option is recorded where it stands rather than answered
			// there: measured, `bash --help --badopt` is the refusal.
			name:   "a refused word behind it still wins",
			opt:    full,
			usage:  usage,
			argv:   []string{"testsh", "--help", "--badopt"},
			errs:   "testsh: --badopt: invalid option\nUsage:\ttestsh [option] ...\n",
			status: 2,
		},
		{
			// A dialect with no block writes the line and the trailer with
			// nothing between them, which is what makes the block the
			// diagnostics' rather than this value's.
			name:   "with no block to write",
			opt:    full,
			argv:   []string{"testsh", "--help"},
			out:    "testsh 1.2.3\nType `testsh -c help' for more.\n",
			status: 0,
		},
		{
			// Each of the three pieces is optional and the status is the
			// dialect's, which is the shape VersionOption already has.
			name:   "on standard error at a failing status",
			opt:    interp.HelpOption{Spellings: "--help", Text: "testsh", ToStandardError: true, Status: 2},
			argv:   []string{"testsh", "--help"},
			errs:   "testsh\n",
			status: 2,
		},
		{
			// The zero value is a shell with no such option — four of the
			// six columns — and the word is refused as any unknown long
			// option is.
			name:   "refused where the dialect names none",
			opt:    interp.HelpOption{},
			usage:  usage,
			argv:   []string{"testsh", "--help"},
			errs:   "testsh: --help: invalid option\nUsage:\ttestsh [option] ...\n",
			status: 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs strings.Builder
			sh := helpShell(c.opt, c.usage)
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
