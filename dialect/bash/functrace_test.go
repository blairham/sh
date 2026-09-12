// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// The trap-carriage options under all three of their spellings — `set -T`,
// `set -o functrace` and `shopt -s extdebug` — measured against bash 5.3.15
// and bash 3.2.57 on 2026-09-12 and recorded in the corpus as the
// `opt/set-o-functrace-…`, `opt/set-o-errtrace-…` and `shopt/extdebug-…`
// cases.
//
// The letters already wrote the state and the long names were refused as
// `not implemented`, which is the pairing #2426 is about: a debugging script
// sets the option, reads 1 from `$?` where every bash column reads 0, and
// then watches a DEBUG trap that sees the *call* and nothing inside it. Two
// failures that compound, and the second is invisible from the first.

// runTraced runs one snippet through the bash dialect the way `-c` does.
func runTraced(t *testing.T, src string) (string, string, int) {
	t.Helper()
	var out, errs bytes.Buffer
	code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", src})
	return out.String(), errs.String(), code
}

// TestTheTraceOptionsAreTakenByName is the first half: the name is accepted,
// silently and at 0, and the listing then reports the state it was asked for.
func TestTheTraceOptionsAreTakenByName(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{"functrace on", `set -o functrace; echo "st=$?"; set -o | grep '^functrace'`, "st=0\nfunctrace      \ton\n"},
		{"errtrace on", `set -o errtrace; echo "st=$?"; set -o | grep '^errtrace'`, "st=0\nerrtrace       \ton\n"},
		// Off again, which is what parts a name that moved from a name that
		// was accepted and recorded nothing.
		{"functrace off again", `set -o functrace; set +o functrace; set -o | grep '^functrace'`, "functrace      \toff\n"},
		// The letter and the name are one state, in both directions: a
		// second table for the long spelling is how the two come apart.
		{"the letter reads back by name", `set -T; set -o | grep '^functrace'`, "functrace      \ton\n"},
		{"the name reads back as a letter", `set -o functrace; case $- in *T*) echo letter;; *) echo none;; esac`, "letter\n"},
		{"errtrace reads back as a letter", `set -o errtrace; case $- in *E*) echo letter;; *) echo none;; esac`, "letter\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runTraced(t, c.src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and nothing said",
					c.src, out, errs, code, c.want)
			}
		})
	}
}

// TestFunctraceCarriesTheDebugTrapIntoACall is the half the status cannot
// see, and the reason the option is worth having: without it the trap
// announces the call and nothing the call does.
func TestFunctraceCarriesTheDebugTrapIntoACall(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		// The bound this shell keeps with the option off: one D, for `f`.
		{"off", `f() { echo in-f; }; trap 'echo D' DEBUG; f`, "D\nin-f\n"},
		{"by name", `f() { echo in-f; }; set -o functrace; trap 'echo D' DEBUG; f`, "D\nD\nin-f\n"},
		{"by letter", `f() { echo in-f; }; set -T; trap 'echo D' DEBUG; f`, "D\nD\nin-f\n"},
		{"by extdebug", `f() { echo in-f; }; shopt -s extdebug; trap 'echo D' DEBUG; f`, "D\nD\nin-f\n"},
		// A subshell is the other boundary the option crosses, and the half
		// a function-only implementation would pass the rows above without.
		// The group itself fires nothing in either state.
		{"a subshell, off", `trap 'echo D' DEBUG; (echo s); echo after`, "s\nD\nafter\n"},
		{"a subshell, on", `set -o functrace; trap 'echo D' DEBUG; (echo s); echo after`, "D\ns\nD\nafter\n"},
		// And the listing follows the firing: an inherited trap that still
		// runs here is still listed here, through a modification that drops
		// the snapshot the other kind is listed from.
		{"an inherited listing", `set -T; trap 'echo D' DEBUG; (trap '' USR2; trap)`, "D\nD\ntrap -- '' SIGUSR2\ntrap -- 'echo D' DEBUG\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runTraced(t, c.src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and nothing said",
					c.src, out, errs, code, c.want)
			}
		})
	}
}

// TestErrtraceCarriesTheErrTrapIntoACall is the ERR half of the same pair,
// and it is a pair: neither option says anything about the other's trap.
func TestErrtraceCarriesTheErrTrapIntoACall(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{"off", `trap 'echo E' ERR; f() { false; }; f; echo done`, "E\ndone\n"},
		{"by name", `set -o errtrace; trap 'echo E' ERR; f() { false; }; f; echo done`, "E\nE\ndone\n"},
		{"a subshell, off", `trap 'echo E' ERR; (false); echo done`, "E\ndone\n"},
		{"a subshell, on", `set -E; trap 'echo E' ERR; (false); echo done`, "E\nE\ndone\n"},
		// functrace is not errtrace: the DEBUG option leaves the ERR trap
		// where the dialect bounds it.
		{"functrace does not move it", `set -o functrace; trap 'echo E' ERR; f() { false; }; f; echo done`, "E\ndone\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runTraced(t, c.src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and nothing said",
					c.src, out, errs, code, c.want)
			}
		})
	}
}

// TestExtdebugIsAnIndicatorAndTwoOptions holds the one state that cannot be
// derived from the other two. Measured in bash 5.3.15: the name turns both
// options on, `shopt -p` writes it back, and `set +T` afterwards leaves the
// indicator on with `functrace` off — so a shell deriving the bit from the
// options would answer that last query wrong.
func TestExtdebugIsAnIndicatorAndTwoOptions(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{"taken and written back", `shopt -s extdebug; echo "st=$?"; shopt -p extdebug`, "st=0\nshopt -s extdebug\n"},
		{"it moves both options", `shopt -s extdebug; set -o | grep -E '^(errtrace|functrace)'`, "errtrace       \ton\nfunctrace      \ton\n"},
		{"unset moves both back", `shopt -s extdebug; shopt -u extdebug; shopt -p extdebug; set -o | grep -E '^(errtrace|functrace)'`, "shopt -u extdebug\nerrtrace       \toff\nfunctrace      \toff\n"},
		// The three come apart, which is why the indicator is stored.
		{"the indicator outlives the option", `shopt -s extdebug; set +T; shopt -p extdebug; set -o | grep '^functrace'`, "shopt -s extdebug\nfunctrace      \toff\n"},
		{"the option does not set the indicator", `set -T; shopt -p extdebug`, "shopt -u extdebug\n"},
		// It belongs to this shell rather than to the one that spawned it.
		{"a subshell keeps it to itself", `(shopt -s extdebug); shopt -p extdebug`, "shopt -u extdebug\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runTraced(t, c.src)
			if out != c.want || errs != "" {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and nothing said",
					c.src, out, errs, code, c.want)
			}
		})
	}
}

// A query of a name that is off answers 1, which is what makes `shopt` a
// reading rather than a formality — and the status a capture harness tests.
func TestExtdebugQueryAnswersByStatus(t *testing.T) {
	if _, _, code := runTraced(t, `shopt -q extdebug`); code != 1 {
		t.Errorf("query of an unset extdebug gave %d, want 1", code)
	}
	if _, _, code := runTraced(t, `shopt -s extdebug; shopt -q extdebug`); code != 0 {
		t.Errorf("query of a set extdebug gave %d, want 0", code)
	}
}

// And the name still appears in the whole-table listing, which is the surface
// a harness sources back: a name wired to real state must not drop out of it.
func TestExtdebugStaysInTheListing(t *testing.T) {
	out, _, _ := runTraced(t, `shopt -p`)
	if !strings.Contains(out, "shopt -u extdebug\n") {
		t.Errorf("`shopt -p` wrote %q, want an extdebug row", out)
	}
}
