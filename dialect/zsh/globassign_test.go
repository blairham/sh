// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `GLOB_ASSIGN` makes the right-hand side of a plain scalar assignment a
// pattern. It was in the option table and nothing read it until #4638:
// `setopt globassign` succeeded, `[[ -o globassign ]]` and the `setopt`
// listing reported it back faithfully, and `a=*.txt` went on storing the six
// characters in either state.
//
// Every case below is run in **both** states for that reason — a shell that
// ignores an option fails one half of every pair, where a row that only ever
// asked it one question can pass without the option being read at all.
//
// Measured against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — with `-f`, over a script file, 2026-09-26.
// `go version -m` on that binary says *not a Go executable*.

// globTree builds the directory every case here runs in:
//
//	a.txt  b.txt  c.txt   three names one pattern reaches
//	one.only              the single match, which is a *scalar*
//	plain                 a name with no metacharacter to be about
//	dir/d1  dir/d2        a match with a slash in the pattern
func globTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a.txt", "b.txt", "c.txt", "one.only", "plain", "dir/d1", "dir/d2"} {
		if err := os.WriteFile(filepath.Join(root, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The grid, in both states. `typeset -p` rather than `print` because the
// *kind* is half the answer: `a=*.txt` does not join the three names into a
// scalar, it makes the name an array, and an echo of `$a` cannot tell those
// apart.
func TestGlobAssignOverTheGridInBothStates(t *testing.T) {
	for _, c := range []struct{ name, src, off, on string }{
		{
			// The issue's own shape. Several matches, and the name comes
			// back an array.
			"several matches", `a=*.txt`,
			"typeset a='*.txt'\n",
			"typeset -a a=( a.txt b.txt c.txt )\n",
		},
		{
			// **One** match is a scalar and not a one-element array, which
			// is the row an implementation that rewrote the line as
			// `a=( … )` gets wrong at status 0.
			"one match", `a=one.*`,
			"typeset a='one.*'\n", "typeset a=one.only\n",
		},
		{
			// No match, and the shell's default `NOMATCH` decides it — the
			// same refusal any other unmatched pattern earns. Nothing is
			// stored, so the name is still the one the line before set.
			"no match", "a=kept\na=*.nomatch",
			"typeset a='*.nomatch'\n", "",
		},
		{
			// `nullglob` is the other half of that row: the word is deleted
			// and the name is left an **empty scalar**, not an empty array.
			"no match under nullglob", "setopt nullglob\na=*.nomatch",
			"typeset a='*.nomatch'\n", "typeset a=''\n",
		},
		{
			// And `unsetopt nomatch` leaves the characters standing, so the
			// miss is the ordinary miss in all three readings.
			"no match with nomatch off", "unsetopt nomatch\na=*.nomatch",
			"typeset a='*.nomatch'\n", "typeset a='*.nomatch'\n",
		},
		{
			"a slash in the pattern", `a=dir/*`,
			"typeset a='dir/*'\n", "typeset -a a=( dir/d1 dir/d2 )\n",
		},
		{
			"a bare star", `a=*`,
			"typeset a='*'\n",
			"typeset -a a=( a.txt b.txt c.txt dir one.only plain )\n",
		},
		{
			// The control that must not move: no metacharacter, nothing to
			// glob, and the option has nothing to do.
			"no metacharacter", `a=plain`,
			"typeset a=plain\n", "typeset a=plain\n",
		},
		{
			// The second control: the metacharacter is there and it is
			// quoted, so it is not a pattern in either state.
			"a quoted pattern", `a='*.txt'`,
			"typeset a='*.txt'\n", "typeset a='*.txt'\n",
		},
		{
			// **The discriminating control**, and the one that says what
			// the rule is keyed on. An array literal's elements are
			// ordinary words and glob in *either* state — so "assignments
			// glob under the option" and "this assignment's words are
			// words" are told apart here rather than anywhere else.
			"an array literal globs in both states", `a=(*.txt)`,
			"typeset -a a=( a.txt b.txt c.txt )\n",
			"typeset -a a=( a.txt b.txt c.txt )\n",
		},
		{
			// A match replaces the name whichever operator was written, so
			// the `x` is gone rather than in front of the first name.
			"append over a scalar", "a=x\na+=*.txt",
			"typeset a='x*.txt'\n", "typeset -a a=( a.txt b.txt c.txt )\n",
		},
		{
			// And the joining `+=` is still there for a value that is not a
			// pattern, which is what keeps the row above from reading as
			// "the option breaks append".
			"append with no pattern", "a=x\na+=plain",
			"typeset a=xplain\n", "typeset a=xplain\n",
		},
		{
			"append over an array", "a=(q w)\na+=*.txt",
			"typeset -a a=( q w '*.txt' )\n",
			"typeset -a a=( a.txt b.txt c.txt )\n",
		},
		{
			// `unsetopt glob` takes the filesystem away from every pattern,
			// this one included.
			"glob off", "unsetopt glob\na=*.txt",
			"typeset a='*.txt'\n", "typeset a='*.txt'\n",
		},
		{
			// An `(N)` qualifier is `nullglob` for one pattern and reaches
			// this road too.
			"an N qualifier", `a=*.nomatch(N)`,
			"typeset a='*.nomatch(N)'\n", "typeset a=''\n",
		},
		{
			"a glob qualifier that matches", `a=*(.)`,
			"typeset a='*(.)'\n",
			"typeset -a a=( a.txt b.txt c.txt one.only plain )\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, setopt, want string }{
				{"off", "unsetopt globassign\n", c.off},
				{"on", "setopt globassign\n", c.on},
			} {
				t.Run(state.name, func(t *testing.T) {
					root := globTree(t)
					out, _ := runZsh(t, root, state.setopt+c.src+"\ntypeset -p a\n")
					if state.want == "" {
						// The refused row: the name never took the pattern,
						// so the listing shows what the line before it set.
						if out != "typeset a=kept\n" && !strings.Contains(out, "no matches found") {
							t.Errorf("got %q, want the refusal and the name unchanged", out)
						}
						return
					}
					if out != state.want {
						t.Errorf("got %q, want %q", out, state.want)
					}
				})
			}
		})
	}
}

// **The noun is the assignment and not the value.** This is the pair the grid
// above cannot produce: the same value, the same directory and the same
// option state, differing only in whether the assignment is a statement of
// its own or a declaration utility's operand.
//
// "a scalar assignment" and "an assignment whose value holds a pattern" agree
// on every row of the grid above except the no-metacharacter control — and
// they part here, where the second reading would have `typeset a=*.txt`
// matching and it does not. Measured on zsh 5.9.2, 2026-09-26, with the
// option **on** throughout.
func TestGlobAssignIsAboutTheAssignmentAndNotTheValue(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a statement", `a=*.txt`, "typeset -a a=( a.txt b.txt c.txt )\n"},
		{"typeset", `typeset a=*.txt`, "typeset a='*.txt'\n"},
		{"declare", `declare a=*.txt`, "typeset a='*.txt'\n"},
		{"export", `export a=*.txt`, "export a='*.txt'\n"},
		{"readonly", `readonly a=*.txt`, "typeset -r a='*.txt'\n"},
		{
			"local",
			"f() { local a=*.txt; typeset -p a; }\nf\na=done",
			"typeset a='*.txt'\ntypeset a=done\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := globTree(t)
			src := "setopt globassign\n" + c.src + "\ntypeset -p a\n"
			if out, _ := runZsh(t, root, src); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// **What makes the word a pattern is not this option**, which is the other
// half of the same question. The option is the gate on the *assignment*; the
// glob marks are the gate on the *value*, and both have to be open.
//
// Measured on zsh 5.9.2, 2026-09-26. The first pair is the one that says a
// value's asterisk is not a written one; the second and third are the two
// ways a script can ask for it to count.
func TestGlobAssignStillNeedsTheValueToBeAPattern(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a value carrying a pattern does not match",
			"v='*.txt'\na=$v", "typeset a='*.txt'\n",
		},
		{
			"the tilde flag asks for it",
			"v='*.txt'\na=${~v}", "typeset -a a=( a.txt b.txt c.txt )\n",
		},
		{
			"globsubst asks for it for every value",
			"setopt globsubst\nv='*.txt'\na=$v",
			"typeset -a a=( a.txt b.txt c.txt )\n",
		},
		{
			// A command substitution's output is a value like any other.
			"a substitution's output does not match",
			`a=$(echo "*.txt")`, "typeset a='*.txt'\n",
		},
		{
			// An escaped metacharacter is not one.
			"an escaped star", `a=\*.txt`, "typeset a='*.txt'\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := globTree(t)
			out, _ := runZsh(t, root, "setopt globassign\n"+c.src+"\ntypeset -p a\n")
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// The numeric attributes, which is the chunk `A06assign.ztst` stops on.
//
// A match takes the attribute off whichever kind the name comes back as, and
// a value with no pattern in it still reaches the arithmetic reader with the
// attribute intact. Those last two rows are the controls that keep this from
// reading as "the option switches arithmetic off". Measured on zsh 5.9.2,
// 2026-09-26.
func TestGlobAssignOverANumericName(t *testing.T) {
	for _, c := range []struct{ name, src, off, on string }{
		{
			"an integer name taking several matches",
			"integer a\na=*.txt", "",
			"typeset -a a=( a.txt b.txt c.txt )\n",
		},
		{
			"an integer name taking one match",
			"integer a\na=one.*", "",
			"typeset a=one.only\n",
		},
		{
			"a float name taking several matches",
			"float a\na=*.txt", "",
			"typeset -a a=( a.txt b.txt c.txt )\n",
		},
		{
			"an integer name taking an expression",
			"integer a\na=2+2", "typeset -i a=4\n", "typeset -i a=4\n",
		},
		{
			"a float name taking an expression",
			"float a\na=2+2",
			"typeset -E a=4.000000000e+00\n", "typeset -E a=4.000000000e+00\n",
		},
		{
			// The issue's own control: a word that *names* a file and holds
			// no metacharacter is not globbed, so the arithmetic reader gets
			// it and answers 0. A rule written on "a word that names a file"
			// breaks this row.
			"an integer name taking a plain word",
			"integer a\na=plain", "typeset -i a=0\n", "typeset -i a=0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, setopt, want string }{
				{"off", "unsetopt globassign\n", c.off},
				{"on", "setopt globassign\n", c.on},
			} {
				t.Run(state.name, func(t *testing.T) {
					root := globTree(t)
					out, st := runZsh(t, root, state.setopt+c.src+"\ntypeset -p a\n")
					if state.want == "" {
						// With the option off the pattern reaches the
						// arithmetic reader, which cannot read it and ends
						// the shell — the shape the issue reported.
						if st == 0 || !strings.Contains(out, "bad ") {
							t.Errorf("got %q at %d, want the arithmetic refusal", out, st)
						}
						return
					}
					if out != state.want {
						t.Errorf("got %q, want %q", out, state.want)
					}
				})
			}
		})
	}
}

// **The moment the option is read is the store and not the parse.** A body
// written while it was off globs when it is called with it on, and an `eval`
// of a string built while it was off globs too — so nothing about the word
// was decided when it was read.
//
// Both directions, because only the pair says it: a rule read at the parse
// would get the first row's answer for the second.
func TestGlobAssignIsReadWhenTheAssignmentRuns(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a function defined off and called on",
			"unsetopt globassign\nf() { a=*.txt; }\nsetopt globassign\nf",
			"typeset -a a=( a.txt b.txt c.txt )\n",
		},
		{
			"a function defined on and called off",
			"setopt globassign\nf() { a=*.txt; }\nunsetopt globassign\nf",
			"typeset a='*.txt'\n",
		},
		{
			"an eval of a string built off",
			"unsetopt globassign\ns='a=*.txt'\nsetopt globassign\neval \"$s\"",
			"typeset -a a=( a.txt b.txt c.txt )\n",
		},
		{
			"an eval of a string built on",
			"setopt globassign\ns='a=*.txt'\nunsetopt globassign\neval \"$s\"",
			"typeset a='*.txt'\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := globTree(t)
			if out, _ := runZsh(t, root, c.src+"\ntypeset -p a\n"); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// The option travels with the vector rather than as a stored bit, so
// `(setopt globassign)` stays in the subshell — and the emulations reach it:
// `emulate sh` and `emulate ksh` both leave it off and both honor a `setopt`
// that follows, while `emulate csh` turns it on. Measured on zsh 5.9.2,
// 2026-09-26.
func TestGlobAssignUnderSubshellsAndEmulations(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a subshell's setopt stays in the subshell",
			"(setopt globassign; a=*.txt; typeset -p a)\na=*.txt\ntypeset -p a",
			"typeset -a a=( a.txt b.txt c.txt )\ntypeset a='*.txt'\n",
		},
		{
			"emulate sh leaves it off",
			"emulate sh\n[[ -o globassign ]] && print on || print off",
			"off\n",
		},
		{
			"emulate ksh leaves it off",
			"emulate ksh\n[[ -o globassign ]] && print on || print off",
			"off\n",
		},
		{
			"emulate csh turns it on",
			"emulate csh\n[[ -o globassign ]] && print on || print off",
			"on\n",
		},
		{
			"emulate sh reaches the reading",
			"emulate sh\nsetopt globassign\na=*.txt\ntypeset -p a",
			"typeset -a a=( a.txt b.txt c.txt )\n",
		},
		{
			"emulate ksh reaches the reading",
			"emulate ksh\nsetopt globassign\na=*.txt\ntypeset -p a",
			"typeset -a a=( a.txt b.txt c.txt )\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := globTree(t)
			if out, _ := runZsh(t, root, c.src+"\n"); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// The trace says what the script did rather than what it typed: a line with
// no parentheses in it writes the element list a written literal would get,
// once the match made the name an array. One match writes the value bare and
// no match writes `”`. Measured on zsh 5.9.2, 2026-09-26, with `setopt
// globassign xtrace`.
func TestGlobAssignIsTracedAsWhatItStored(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"several matches", "a=*.txt", "> a=( a.txt b.txt c.txt ) "},
		{"one match", "a=one.*", "> a=one.only "},
		{"no match under nullglob", "setopt nullglob\na=*.nomatch", "> a='' "},
		{"no pattern", "a=plain", "> a=plain "},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := globTree(t)
			out, _ := runZsh(t, root, "setopt globassign xtrace\n"+c.src+"\n")
			if !strings.Contains(out, c.want) {
				t.Errorf("got %q, want a line ending %q in it", out, c.want)
			}
		})
	}
}
