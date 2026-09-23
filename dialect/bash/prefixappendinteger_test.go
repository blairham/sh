// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `+=` in a command's **prefix** asks the name's attributes, exactly as `+=` on
// a line of its own does.
//
// One spelling over two operations and the *name* says which: a plain name joins
// the characters and a name carrying the integer or float attribute adds. The
// statement form had asked since that helper existed; the prefix form was a
// second concatenation written beside it and did not — measured 2026-09-22 on
// bash 5.3.20, `typeset -i x=2; x+=5 printenv x` is `7` and this shell wrote
// `25`, a wrong number at status 0 (#4142).
//
// Found as three lines of `appendop.tests`, where the same append is read by an
// `eval` the prefix stands in front of.
func TestAPrefixAppendAsksTheNamesAttributes(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		// The integer letter adds, and the prefix's value is what the command
		// is handed — the shell's own copy is untouched, which is the other
		// half of what a prefix assignment means.
		{"integer", `typeset -i x=2; f(){ echo "in:$x"; }; x+=5 f; echo "after:$x"`, "in:7 after:2"},
		{"a plain name still joins", `x=a; f(){ echo "in:$x"; }; x+=b f; echo "after:$x"`, "in:ab after:a"},
		// The right-hand side is an expression rather than a numeral, which is
		// the same thing the statement form reads.
		{"an expression", `typeset -i x=2; f(){ echo "in:$x"; }; x+=3*4 f`, "in:14"},
		// And the base is the name's, not the value's.
		{"a hexadecimal addend", `typeset -i x=1; f(){ echo "in:$x"; }; x+=0x10 f`, "in:17"},
		// A name with no integer letter and a numeric value still joins, which
		// is what keeps this about the attribute rather than about the digits.
		{"digits without the letter", `x=2; f(){ echo "in:$x"; }; x+=5 f`, "in:25"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, dir, c.src)
			if got := strings.Join(strings.Fields(out), " "); got != c.want {
				t.Errorf("output %q, want %q", out, c.want)
			}
		})
	}
}

// A prefix whose append will not evaluate is complained about **once**.
//
// The value is computed twice on the way to a command — once for what the
// command is handed and once for what the shell keeps — so a join that
// evaluates is a join that can report twice. Measured: `typeset -i x=1; x+=2+ f`
// is one `arithmetic syntax error` in bash 5.3.20, and the moment the prefix
// started asking the attribute it was two here.
func TestAFailedPrefixAppendIsComplainedAboutOnce(t *testing.T) {
	dir := t.TempDir()
	out, _ := runBash(t, dir, `typeset -i x=1; f(){ echo ran; }; x+=2+ f; echo tail`)
	if n := countLines(out, `bash: line 1: 2+: arithmetic syntax error: operand expected (error token is "+")`); n != 1 {
		t.Errorf("output %q has the complaint %d times, want once", out, n)
	}
	// And it ends the input, which is what says the second reading never
	// happened rather than having been silenced. Whole lines, not a substring:
	// the complaint itself contains `operand`, so a Contains check for `ran`
	// fails on the very sentence it is meant to look past.
	for _, line := range strings.Split(out, "\n") {
		if line == "ran" || line == "tail" {
			t.Errorf("output %q, want nothing after the refusal", out)
		}
	}
}
