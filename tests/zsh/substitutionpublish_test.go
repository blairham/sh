// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// What a reporting substitution leaves in `$match`, `$mbegin` and `$mend`, and
// what its replacement can see of them while it runs (#6380).
//
// A replacement that expands nothing cannot read the parameters, so only the
// last match's report is observable and the shell publishes it once, when the
// walk is done. One that expands anything sees each match's report before it
// is read. Measured on zsh 5.9.2, 2026-10-11, from script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME and ZDOTDIR.
func TestASubstitutionPublishesWhatItsReplacementCanSee(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, want string }{
		// Inert replacements: the last match is what is left behind.
		{`x=abc; print -r -- ${x//(#b)([a-c])/X}; print -r -- $match $mbegin $mend`, "XXX\nc 3 3\n"},
		{`x=abc; print -r -- ${x//(#b)([a-c])/"X"}; print -r -- $match`, "XXX\nc\n"},
		{`x=abc; print -r -- ${x//(#m)[a-c]/X}; print -r -- $MATCH $MBEGIN $MEND`, "XXX\nc 3 3\n"},
		{`x=abc; print -r -- ${x//(#b)([a-c])}; print -r -- $match`, "\nc\n"},
		// A list publishes per element and leaves the last element's match.
		{`a=(abc xbz); print -r -- ${a//(#b)(b)/X}; print -r -- $match $mbegin`, "aXc xXz\nb 2\n"},
		// A replacement that reads them sees each match in turn.
		{`x=abc; print -r -- ${x//(#b)([a-c])/$match[1]$mbegin[1]}`, "a1b2c3\n"},
		{`x=abc; print -r -- ${x//(#m)[a-c]/<$MATCH:$MBEGIN>}`, "<a:1><b:2><c:3>\n"},
		// And `(e)` reads them after the walk, where the last is left.
		{`x=abc; print -r -- ${(e)x//(#b)([a-c])/\$match[1]}`, "ccc\n"},
		// A function's local copy takes the report and the global keeps its own.
		{`f() { local match; x=abc; print -r -- ${x//(#b)([a-c])/X}; print in:$match; }; match=(g); f; print out:$match`, "XXX\nin:c\nout:g\n"},
	} {
		src := "setopt extendedglob; " + c.src
		if out, st := runZsh(t, t.TempDir(), src); out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}

// A read-only `match` refuses the write once, not once per match: measured on
// zsh 5.9.2, `readonly match; x=aaa; ${x//(#b)(a)/X}` prints one
// `read-only variable: match` and gives up the line, as does the same with
// `(#m)` and `MATCH`. This shell printed it three times while it published
// every match.
func TestAReadOnlyReportIsRefusedOnce(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, name string }{
		{`readonly match; x=aaa; print -r -- ${x//(#b)(a)/X}; print after`, "match"},
		{`readonly MATCH; x=aaa; print -r -- ${x//(#m)a/X}; print after`, "MATCH"},
	} {
		out, st := runZsh(t, t.TempDir(), "setopt extendedglob; "+c.src)
		if n := strings.Count(out, "read-only variable: "+c.name); n != 1 || strings.Contains(out, "after") || st == 0 {
			t.Errorf("%s: %q (status %d), want one refusal and nothing after it", c.src, out, st)
		}
	}
}

// The same pattern text read two ways in one script, which is the case the
// facts kept per pattern must keep apart: `@(a)` is a group under `kshglob`
// and an `@` before one without it. Measured on zsh 5.9.2.
func TestOnePatternReadTwoWaysReportsEachReading(t *testing.T) {
	t.Parallel()
	src := `setopt extendedglob; x=ab; y=@ab
f() { print -r -- "${x/(#b)@(a)(b)/<$match>}" "${y/(#b)@(a)(b)/<$match>}"; }
f; setopt kshglob; f; unsetopt kshglob; f`
	want := "ab <a b>\n<a b> @<a b>\nab <a b>\n"
	if out, st := runZsh(t, t.TempDir(), src); out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
