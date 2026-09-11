// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// The two kinds of alias one dialect has, measured against zsh 5.9.2 and
// pinned here as the parser's half of them (#2081).
//
// The builtin that defines them is interp's; this is the substitution, and it
// is the half that cannot be moved to execution time — a global alias may
// hold a pipe, a separator or a reserved word, and a suffix alias replaces the
// word before anything has looked for a command by that name.

// kinds parses src with all three tables and renders what the parser made of
// it, so a test can say what the expansion came to.
func kinds(t *testing.T, src string, global, suffix syntax.Aliases, plain syntax.Aliases) string {
	t.Helper()
	p := syntax.NewParser(src, syntax.Core())
	p.Aliases = plain
	p.GlobalAliases = global
	p.SuffixAliases = suffix
	f := p.Parse()
	if err := p.Err(); err != nil {
		return "error: " + err.Error()
	}
	return strings.TrimSpace(syntax.Print(f))
}

// **A global alias is expanded wherever a word stands.** Every row was
// measured one at a time; the ones that do *not* expand are what make it a
// rule rather than a replacement of every occurrence.
func TestAGlobalAliasExpandsWhereverAWordStands(t *testing.T) {
	g := table("G", "one two", "UP", "| tr a-z A-Z", "SEP", ";", "RO", ">", "TH", "then", "F", "outfile")
	for _, c := range []struct{ name, src, want string }{
		{"an argument", "echo a G b", "echo a one two b"},
		{"command position too", "G", "one two"},
		{"a for list", "for x in G; do echo $x; done", "for x in one two; do echo $x; done"},
		{"a redirection target", "echo x > F", "echo x > outfile"},
		{"a pipeline out of a value", "echo hi UP", "echo hi | tr a-z A-Z"},
		{"a separator out of a value", "echo one SEP echo two", "echo one; echo two"},
		{"a redirection operator out of a value", "echo hi RO out", "echo hi > out"},
		{"a reserved word out of a value", "if true; TH echo yes; fi", "if true; then echo yes; fi"},
		// The negative half.
		{"not quoted with double quotes", `echo "G"`, `echo "G"`},
		{"not quoted with single quotes", "echo 'G'", "echo 'G'"},
		{"not inside a quoted word", `echo "x G y"`, `echo "x G y"`},
		{"not an assignment, which is one word", "v=G", "v=G"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := kinds(t, c.src, g, nil, nil); got != c.want {
				t.Errorf("%q = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// **The recursion rule is not the regular kind's.** The names a word spends
// are its own, so the same global alias twice in one command expands twice —
// where a regular alias in one command is spent after the first. A cycle
// still stops, at the name it started on.
func TestAGlobalAliasSpendsItsNamesPerWord(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		g               syntax.Aliases
	}{
		{"twice in one command", "printf S S", "printf x x", table("S", "x")},
		{"a self-reference stops", "printf S", "printf S x", table("S", "S x")},
		{"a cycle stops where it began", "printf A", "printf A", table("A", "B", "B", "A")},
		{"nested inside a body", "printf H", "printf a x", table("B", "x", "H", "a B")},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := kinds(t, c.src, c.g, nil, nil); got != c.want {
				t.Errorf("%q = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// **And what a word spends is spent for the command word too**, which is the
// half that keeps the two sets from fighting: `alias -g f='echo f'` run as a
// command prints `f` rather than recurring. The table is passed as both the
// plain and the global one, which is what the runner does — a global alias
// answers in command position as well.
func TestAGlobalAliasInCommandPositionIsSpentOnce(t *testing.T) {
	g := table("f", "echo f")
	if got, want := kinds(t, "f", g, nil, g), "echo f"; got != want {
		t.Errorf("f = %q, want %q", got, want)
	}
}

// **A suffix alias replaces the command word with `value word`.**
func TestASuffixAliasReplacesTheCommandWord(t *testing.T) {
	s := table("txt", "cat", "sh", "echo SUFFIX", "ps", "gv --")
	for _, c := range []struct{ name, src, want string }{
		{"a bare name", "x.txt", "cat x.txt"},
		{"a relative path", "./x.txt", "cat ./x.txt"},
		{"an absolute path", "/tmp/x.txt", "cat /tmp/x.txt"},
		{"a dot inside a directory name", "a/.txt", "cat a/.txt"},
		{"the last dot decides", "a.b.sh", "echo SUFFIX a.b.sh"},
		{"arguments follow the word", "./x.txt one", "cat ./x.txt one"},
		{"a glob is replaced before it globs", "*.ps", "gv -- *.ps"},
		// The negative half.
		{"text must be non-empty", ".txt", ".txt"},
		{"the run after the last dot is the whole of the name", "q.sh/w", "q.sh/w"},
		{"no dot at all", "plain", "plain"},
		{"a quoted word is not one", "'./x.txt'", "'./x.txt'"},
		{"an unknown extension", "./y.dat", "./y.dat"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := kinds(t, c.src, nil, s, nil); got != c.want {
				t.Errorf("%q = %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// **A value holding a pipeline puts the word after its last command**, which
// is what says the substitution is of text rather than of one word for
// another. Measured: `alias -s txt='cat | tr a-z A-Z'` hands `./x.txt` to
// `tr`, not to `cat`.
func TestASuffixAliasValueIsText(t *testing.T) {
	s := table("txt", "cat | tr a-z A-Z")
	if got, want := kinds(t, "./x.txt", nil, s, nil), "cat | tr a-z A-Z ./x.txt"; got != want {
		t.Errorf("./x.txt = %q, want %q", got, want)
	}
}

// **A regular alias of the whole word wins**, and an assignment prefix does
// not move the command word.
func TestASuffixAliasIsTriedAfterTheTable(t *testing.T) {
	s := table("sh", "echo SUFFIX")
	if got, want := kinds(t, "p.sh", nil, s, table("p.sh", "echo ALIAS")), "echo ALIAS"; got != want {
		t.Errorf("with a regular alias = %q, want %q", got, want)
	}
	if got, want := kinds(t, "v=1 p.sh", nil, s, nil), "v=1 echo SUFFIX p.sh"; got != want {
		t.Errorf("past an assignment prefix = %q, want %q", got, want)
	}
}

// **An empty value cannot loop**, which is the reason the word goes into the
// spent set rather than only the suffix: the splice leaves the same word
// standing where a command word stands, and matching it again would never
// end.
func TestASuffixAliasWithAnEmptyValueTerminates(t *testing.T) {
	if got, want := kinds(t, "./x.txt", nil, table("txt", ""), nil), "./x.txt"; got != want {
		t.Errorf("./x.txt = %q, want %q", got, want)
	}
}

// **Neither kind reaches a parser that was given no table**, which is every
// dialect but one. Written as a test because the hooks are read on the
// keystroke path and a nil check that stopped working would be invisible.
func TestNoTableMeansNoExpansion(t *testing.T) {
	for _, src := range []string{"echo G", "./x.txt", "G"} {
		if got := kinds(t, src, nil, nil, nil); got != src {
			t.Errorf("%q with no tables = %q, want it untouched", src, got)
		}
	}
}
