// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The `function` keyword with no name takes the **next command** as its body,
// and what it makes is a *call* (#5078).
//
// This engine ran nothing for the keyword and left the command under it
// standing on its own, which prints the same bytes for any body that prints
// something — so the whole of the difference is in what the body can see.
//
// Measured 2026-09-28 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*), from script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME and standard input
// on the null device.
func TestTheKeywordTakesTheNextCommandAsItsBody(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		// The three discriminators. Each holds the printed text fixed and
		// moves only what the body is allowed to know.
		{
			"the body reports the invented name",
			`function; printf "0=%s" $0`, "0=(anon)",
		},
		{
			"a declaration in the body does not outlive it",
			`function; typeset y=2; printf "[%s]" $y`, "[]",
		},
		{
			"and the body's positional parameters are the call's",
			`set -- p q; function; printf "n=%d" $#`, "n=0",
		},
		// The separator may be a newline as well as a `;`, and blanks,
		// comments and joined lines stand between the two.
		{"over a newline", "function\n" + `printf "0=%s" $0`, "0=(anon)"},
		{"over a blank line", "function\n\n" + `printf "0=%s" $0`, "0=(anon)"},
		{"over a comment", "function\n# why\n" + `printf "0=%s" $0`, "0=(anon)"},
		// Not a joined line, and this is the row that says the separator
		// is a *terminator* rather than whitespace: a `\` before the
		// newline joins the two lines, so `printf` is on the keyword's own
		// line and is read as the name this form does not have. Nothing
		// runs and nothing is printed. The bracketed spelling survives the
		// join because `{` is a body wherever it stands.
		{"a joined line makes the word a name", "function \\\n" + `printf "0=%s" $0`, ""},
		{"where the bracketed body survives it", "function \\\n" + `{ printf "0=%s" $0 }`, "0=(anon)"},
		// Any command, not only a simple one.
		{"a loop is a body", `function; for i in a; do printf "0=%s" $0; done`, "0=(anon)"},
		{"an `if` is a body", `function; if true; then printf "0=%s" $0; fi`, "0=(anon)"},
		// And the keyword itself, which is how two of them nest.
		{
			"and so is another bare keyword",
			"function\nfunction\n" + `printf "0=%s" $0`, "0=(anon)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The body is a whole **and-or list**, not one pipeline.
//
// The row that says so cannot be read off the first one: with `print` on both
// sides of an `&&`, a body of one command and a body of two print the same
// bytes, and only a name's lifetime separates them. `typeset` in the left half
// and a read of it in the right is the pair — one call means the right half
// sees the name and the line after does not.
func TestTheImpliedBodyIsAWholeAndOrList(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"both halves of an `&&` are inside one call",
			"function\n" + `typeset y=1 && printf "in=[%s] " $y` + "\n" + `printf "after=[%s]" $y`,
			"in=[1] after=[]",
		},
		{
			"and the brace spelling of the same program agrees",
			`function { typeset y=1 && printf "in=[%s] " $y }` + "\n" + `printf "after=[%s]" $y`,
			"in=[1] after=[]",
		},
		{
			"an `||` reaches its right half too",
			"function\n" + `false || printf "0=%s" $0`, "0=(anon)",
		},
		// The bracketed spelling is the other way round, and this is the
		// row that keeps the two from being made the same: only the first
		// command goes inside there.
		{
			"but the bracketed header takes one command only",
			`() printf "a=%s " $0 && printf "b=%s" $0`, "a=(anon) b=zsh",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The keyword takes a body only where it **ended its own command**.
//
// A redirection, a pipe or an `&&` written after it leaves it nothing to take,
// and the command on the line below is the script's own. These are the rows
// that say the reach is bounded by the grammar rather than by the next
// non-blank byte.
func TestTheKeywordTakesNothingWhereItDidNotEndItsCommand(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a redirection ends it, and the next line is the script's",
			"function >f\n" + `printf "0=%s " $0` + "\n" + `printf "[%s]" "$(<f)"`,
			"0=zsh []",
		},
		{
			"an `&&` ends it",
			"function && printf \"0=%s\" $0\n", "0=zsh",
		},
		{
			"and so does a pipe",
			"function | cat\n" + `printf "0=%s" $0`, "0=zsh",
		},
		// The mirror image: the keyword at the *end* of a pipeline or an
		// and-or has ended its own command, and the next line is its body.
		{
			"but the last element of a pipeline takes one",
			// `a` is written into the pipe and the keyword drops it, so
			// the only output is the body's.
			"printf a | function\n" + `printf "0=%s" $0`, "0=(anon)",
		},
		{
			"and so does an and-or's right-hand side",
			"false || function\n" + `printf "0=%s" $0`, "0=(anon)",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZshOnPath(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
