// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strconv"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A compound command's redirection is the second road into a process
// substitution, and it has to remove what it made.
//
// `while read x; do :; done < <(cmd)` is the idiom bash users write for a
// loop that must not run in a subshell, and every execution of it expands a
// `<(cmd)`. The removal lived on the simple command alone, so this road made
// a pipe and let go of nothing: one descriptor per execution, which is one
// per *iteration* wherever the loop is itself inside one.
//
// Measured 2026-09-21 against bash 5.3.20, `ulimit -n 64` and 300 turns of
// the idiom: bash ran all 300 and this shell stopped at 59 with `too many
// open files` and then `<(echo x): ambiguous redirect`. It is bash's own
// `tests/redir10.sub` — a file named for this shape, which has been failing
// the same way — and it is what moved that row of `make bash-suite`.
//
// # Why the region and not a count
//
// The assertion this file's neighbor explains: the numbers come from the
// kernel's table for the whole test binary, so a parallel test that opens a
// file moves them and a count of open descriptors measures the suite. What
// repetition may not do is push a substitution **out of the region its rule
// allocates from** — the descent from 63 is some sixty numbers deep, so a
// leak of one per turn walks off the end of it well inside this loop and
// parkDescriptor's floor then answers far above 63. Without the leak the
// number is free every time and the last turn is in the region like the
// first.
func TestACompoundRedirectionsSubstitutionIsRemovedWithIt(t *testing.T) {
	const turns = 80
	out := runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable,
		AllocateDescriptorsFromTen, nil, `
			n=0
			while [ $n -lt `+strconv.Itoa(turns)+` ]; do
				{ true; } < <(true)
				n=$((n+1))
			done
			echo <(true)`, nil)
	got := substNumbers(t, out)
	if len(got) != 1 {
		t.Fatalf("got %v, want one number", got)
	}
	if got[0] > 63 {
		t.Errorf("got /dev/fd/%d after %d compound redirections, want a number still "+
			"at or below the top of the table — each of those turns kept its pipe",
			got[0], turns)
	}
}

// And the pipe still reaches the commands the compound runs, which is the
// half a removal put in the wrong place would take away.
//
// The removal is scoped the way the simple command's is: what the compound
// made is handed on as *enclosing* for the length of its body, so a command
// inside it is given the descriptor by number. `while read x; do echo got:$x;
// done < <(echo hello)` is `got:hello` in bash 5.3.20.
func TestACompoundsSubstitutionReachesItsBody(t *testing.T) {
	out := runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable,
		AllocateDescriptorsFromTen, nil,
		`while read x; do echo got:$x; done < <(echo hello)`, nil)
	if got := strings.TrimSpace(out); got != "got:hello" {
		t.Errorf("got %q, want %q", got, "got:hello")
	}
}

// And a `>(cmd)` written on a compound command produces its output at all.
//
// The removal is what closes the shell's end of a writing substitution's
// pipe, and closing it is the end-of-file its body is reading until — the
// reason the simple command's removal sits where it does rather than beside
// the exec. A compound command had no removal, so the body waited for an end
// that was never coming and the substitution simply produced nothing:
// `{ echo hi; } > >(tr a-z A-Z); echo after` was `after` alone here and is
// `after` then `HI` in bash 5.3.20. Silent, and the shape of the fault the
// simple command's comment already describes, reached by the other road.
//
// The body is a builtin rather than that `tr`, because a Runner in a test has
// no PATH and a body that cannot be found writes nothing either — which is
// the answer this case is looking for, and would have passed for the wrong
// reason.
func TestAWritingSubstitutionOnACompoundProduces(t *testing.T) {
	out := runSubstPlacement(t, SubstitutionEndsAtTheTopOfTheTable,
		AllocateDescriptorsFromTen, nil,
		`{ echo hi; } > >(read x; echo body:$x); echo after`, nil)
	got := strings.Fields(out)
	if len(got) != 2 || got[0] != "after" || got[1] != "body:hi" {
		t.Errorf("got %q, want the command's line and then the body's", out)
	}
}
