// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// **The stage is a second question, and it is a flag.** `for $n in a b` is
// refused by every shell in the panel — that is the predicate, tested beside
// this — and four of the six *parse* it and complain only when the loop is
// reached, which changes three observable things.
//
// Measured 2026-09-06 and re-measured 2026-09-07, `env -i
// PATH=/usr/bin:/bin` with a scratch HOME, over a script file holding `n=x`,
// `for $n in a b; do echo body; done` and `echo "reached-after st=$?"`:
//
//	shell        -n over that file   what a run prints            script status
//	bash 5.3.15  accepts, silent, 0  the complaint, reached st=1  0
//	bash-as-sh   accepts, silent, 0  the complaint, and stops     2
//	bash 3.2.57  accepts, silent, 0  the complaint, reached st=1  0
//	dash         refuses, 2          the complaint, and stops     2
//	ksh93u+      accepts, silent, 0  the complaint, and stops      1
//	zsh 5.9.2    refuses, 1          the complaint, and stops      1
//
// A syntax check is what a CI job runs, so refusing the parse reported a
// working script as broken. What happens when the loop *is* reached is
// interp.Semantics.ForNameWhenTheLoopRuns and not this flag (#1110).

// stagedLoops is loops() with the stage flag on, which is bash and ksh93.
func stagedLoops() syntax.Dialect {
	d := loops()
	d.ForNameCheckedWhenTheLoopRuns = true
	return d
}

// The parse succeeds and the word is carried on the clause, in all three
// spellings of the header.
func TestAnUnusableLoopNameParsesUnderTheFlag(t *testing.T) {
	d := stagedLoops()
	for _, tc := range []struct{ src, want string }{
		{"for $n in a b; do :; done", "$n"},
		{"for 1x in a b; do :; done", "1x"},
		{"for ${n} in a b; do :; done", "${n}"},
		{"for $(echo n) in a b; do :; done", "$(echo n)"},
		{"for \"a b\" in a b; do :; done", `"a b"`},
		{"foreach 1x (a b)\n:\nend", "1x"},
	} {
		f, err := syntax.Parse(tc.src+"\n", d)
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		c := forClauseOf(t, f, tc.src)
		if c.RefusedName != tc.want {
			t.Errorf("%q: RefusedName = %q, want %q", tc.src, c.RefusedName, tc.want)
		}
		// And no name to bind, which is the half a second field would have
		// let drift: a clause carrying one would be a clause the
		// interpreter could try to run.
		if len(c.Names) != 0 {
			t.Errorf("%q: Names = %v, want none", tc.src, c.Names)
		}
	}
	// The menu loop, whose field is its own.
	f, err := syntax.Parse("select $n in a b; do :; done\n", d)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	sc, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SelectClause)
	if !ok {
		t.Fatalf("select: not a select clause")
	}
	if sc.RefusedName != "$n" || sc.Name != "" {
		t.Errorf("select: RefusedName = %q, Name = %q, want `$n` and none", sc.RefusedName, sc.Name)
	}
}

// Without the flag the refusal stays where it was, which is the other half of
// the same claim: the predicate did not move, only the stage did.
func TestAnUnusableLoopNameIsStillRefusedWithoutTheFlag(t *testing.T) {
	for _, src := range []string{
		"for $n in a b; do :; done",
		"for 1x in a b; do :; done",
		"select $n in a b; do :; done",
	} {
		if _, err := syntax.Parse(src+"\n", loops()); err == nil {
			t.Errorf("%q parsed without the flag, want the refusal kept", src)
		}
	}
}

// A usable name is unaffected by the flag, in either direction — the point
// being that nothing about an ordinary loop reads it.
func TestTheFlagDoesNotTouchAUsableName(t *testing.T) {
	for _, d := range []syntax.Dialect{loops(), stagedLoops()} {
		f, err := syntax.Parse("for i in a b; do :; done\n", d)
		if err != nil {
			t.Fatalf("%v", err)
		}
		c := forClauseOf(t, f, "for i")
		if len(c.Names) != 1 || c.Names[0] != "i" || c.RefusedName != "" {
			t.Errorf("Names = %v, RefusedName = %q, want [i] and none", c.Names, c.RefusedName)
		}
	}
}

// **A word only.** `for ; in a b` stays an ordinary unexpected-token failure
// under the flag, because what those two shells want in that position is a
// word and the *name* is what they check later — measured, bash reports a
// syntax error at 2 with the line echoed and ksh93 one at 3.
//
// So the two questions stay apart: whether a word may stand there is the
// grammar's, and whether the word is a name is the loop's. Without this the
// flag would have carried a `;` to the interpreter as a refused name.
func TestANonWordInTheNamePositionIsStillASyntaxError(t *testing.T) {
	d := stagedLoops()
	for _, src := range []string{
		"for ; in a b; do :; done",
		"for | in a b; do :; done",
		"for do :; done",
		"select ; in a b; do :; done",
	} {
		if _, err := syntax.Parse(src+"\n", d); err == nil {
			t.Errorf("%q parsed, want an unexpected-token refusal", src)
		}
	}
}

// The printer writes a refused name back as it stood, so a clause that parses
// and will fail when it runs fails the same way after a round trip.
func TestARefusedNamePrintsBack(t *testing.T) {
	d := stagedLoops()
	for _, src := range []string{
		"for $n in a b; do :; done",
		"for 1x in a b; do :; done",
		"select $n in a b; do :; done",
	} {
		f, err := syntax.Parse(src+"\n", d)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		got := syntax.Print(f)
		again, err := syntax.Parse(got, d)
		if err != nil {
			t.Errorf("%q printed as %q, which does not parse: %v", src, got, err)
			continue
		}
		if syntax.Print(again) != got {
			t.Errorf("%q: printed %q, reprinted %q", src, got, syntax.Print(again))
		}
	}
}

// forClauseOf digs the loop out of a file, failing rather than returning an
// error: every row above has already established that the source parses.
func forClauseOf(t *testing.T, f *syntax.File, src string) *syntax.ForClause {
	t.Helper()
	c, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.ForClause)
	if !ok {
		t.Fatalf("%q: not a for clause", src)
	}
	return c
}

// A loop with **more than one** name and the stage flag together, which no
// shell in the panel is: multiple names are zsh's and zsh refuses the word
// while parsing. So the answer below is this repository's own, and it is
// written down because the two flags are independent and the code reads both
// — a mutant that let a later refused word replace an earlier one survived
// every row until this test existed.
//
// The rule is the one a diagnostic needs: the **first** word that is not a
// name is the one the complaint will quote, and the names before it are still
// read. A later bad word does not displace it.
func TestTheFirstRefusedNameIsTheOneKept(t *testing.T) {
	d := stagedLoops()
	d.ForMultipleNames = true
	for _, tc := range []struct {
		src   string
		want  string
		names []string
	}{
		{"for a 1x in p q; do :; done", "1x", []string{"a"}},
		{"for 1x a in p q; do :; done", "1x", []string{"a"}},
		// Two bad ones: the first is kept, which is the half the guard
		// decides and the half a mutant could otherwise flip unseen.
		{"for 1x 2y in p q; do :; done", "1x", nil},
		{"for a 1x 2y in p q; do :; done", "1x", []string{"a"}},
	} {
		f, err := syntax.Parse(tc.src, d)
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		c := forClauseOf(t, f, tc.src)
		if c.RefusedName != tc.want {
			t.Errorf("%q: RefusedName = %q, want %q", tc.src, c.RefusedName, tc.want)
		}
		if len(c.Names) != len(tc.names) {
			t.Errorf("%q: Names = %v, want %v", tc.src, c.Names, tc.names)
			continue
		}
		for i, n := range tc.names {
			if c.Names[i] != n {
				t.Errorf("%q: Names = %v, want %v", tc.src, c.Names, tc.names)
				break
			}
		}
	}
	// Without the stage flag the same input is refused while parsing, which
	// is what every shell that has multiple names actually does.
	plain := loops()
	plain.ForMultipleNames = true
	if _, err := syntax.Parse("for a 1x in p q; do :; done", plain); err == nil {
		t.Error("`for a 1x` parsed without the stage flag, want the refusal kept")
	}
}
