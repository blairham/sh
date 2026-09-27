// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/blairham/sh/interp"
)

// TestTheTwoSuppressionOptionsAreTwoFacts pins what the front end hands the
// runner when an invocation names one of the startup-file escape hatches.
//
// Two options and two fields, because a dialect's option namespace publishes
// them separately and an invocation may write both. It is the *asked for*
// rather than a tally of the files read, which is what the last two rows say:
// suppressing every file suppresses the machine's as well, and the narrower
// fact still reads no — a shell that derived one from the other would have to
// answer yes there, and the shell this models answers no.
//
// Measured 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh`, `env -i` with an empty `HOME` and `ZDOTDIR` so no
// startup file could speak for either side:
//
//	zsh    -c 'setopt'    nohashdirs
//	zsh -d -c 'setopt'    noglobalrcs nohashdirs
//	zsh -f -c 'setopt'    nohashdirs norcs
//	zsh -f -d -c 'setopt' noglobalrcs nohashdirs norcs
//
// The spellings are the fixture's, not this front end's — see
// interp.StartupFileOptions, which is why this test names the options and
// never the shell.
func TestTheTwoSuppressionOptionsAreTwoFacts(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{"nothing asked", []string{"testsh"}, "all=false system=false\n"},
		{"the narrow one", []string{"testsh", "-d"}, "all=false system=true\n"},
		{"its long spelling", []string{"testsh", "--no-globalrcs"}, "all=false system=true\n"},
		{"the wide one", []string{"testsh", "-f"}, "all=true system=false\n"},
		{"both", []string{"testsh", "-f", "-d"}, "all=true system=true\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanStartupEnv(t)
			sh := shell()
			sh.Semantics = zshLikeSystem()
			sh.Register = func(r *interp.Runner) {
				r.Register("suppressed", func(r *interp.Runner, _ context.Context, _ []string) int {
					_, _ = fmt.Fprintf(r.Out(), "all=%v system=%v\n",
						r.StartupFilesSuppressed, r.SystemStartupFilesSuppressed)
					return 0
				})
			}
			out, errs, code := runArgs(t, sh, append(tc.argv, "-c", "suppressed")...)
			if out != tc.want || errs != "" || code != 0 {
				t.Errorf("%v said %q / %q at %d, want %q and nothing said",
					tc.argv[1:], out, errs, code, tc.want)
			}
		})
	}
}
