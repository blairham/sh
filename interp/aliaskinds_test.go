// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The builtin's half of the two kinds one dialect has (#2081). The parser's
// half — where each is substituted — is in syntax/aliaskinds_test.go.
//
// Measured against zsh 5.9.2. Every listing here is an exact-byte assertion,
// because a listing is a measured artifact and not a formatting choice.

// kindsOn answers both axes yes, which is the dialect that has them.
func kindsOn(s *Semantics) {
	s.GlobalAliases = Yes
	s.SuffixAliases = Yes
	s.AliasListsAsDefinitions = Yes
	s.AliasRestrictsToRegularKind = Yes
	s.AliasOperandsCanBePatterns = Yes
	s.AliasPlusPrintsNamesOnly = Yes
}

func kindsRun(t *testing.T, src string) (string, int) {
	t.Helper()
	return aliasRun(t, kindsOn, Diagnostics{}, src)
}

// **A plain `alias` lists the regular and the global kind together, `-g`
// lists the global alone, and `-s` the suffix alone** — sorted, and never
// mixed. That is the whole of "two namespaces" as a listing.
func TestTheThreeListingsAreThreeSets(t *testing.T) {
	out, st := kindsRun(t, `alias -g G='b b'; alias r=y; alias -s t=z
echo "--plain--"; alias
echo "--g--"; alias -g
echo "--s--"; alias -s`)
	want := "--plain--\nG='b b'\nr='y'\n--g--\nG='b b'\n--s--\nt='z'\n"
	if out != want || st != 0 {
		t.Errorf("the three listings = %q (status %d), want %q", out, st, want)
	}
}

// **A global alias replaces a regular one of the same name**, because they
// share a table — and a suffix alias of the same name does not, because it
// does not.
func TestAGlobalAliasReplacesARegularOne(t *testing.T) {
	out, st := kindsRun(t, `alias dup=REGULAR; alias -g dup=GLOBAL; alias -s dup=SUFFIX
echo "--plain--"; alias
echo "--s--"; alias -s`)
	want := "--plain--\ndup='GLOBAL'\n--s--\ndup='SUFFIX'\n"
	if out != want || st != 0 {
		t.Errorf("one name in two namespaces = %q (status %d), want %q", out, st, want)
	}
}

// **A named operand is found by the table and printed by the letter.**
// Measured, and the surprising row is the third: `alias -g` naming a regular
// alias reports success and says nothing, because the table holds the name
// and the letter is a filter on what gets written.
func TestANamedOperandIsFoundByTheTable(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"a global under -g", `alias -g G=x; alias -g G`, "G='x'\n", 0},
		{"a global under the plain form", `alias -g G=x; alias G`, "G='x'\n", 0},
		{"a regular under -g", `alias r=y; alias -g r`, "", 0},
		{"a suffix under -s", `alias -s t=z; alias -s t`, "t='z'\n", 0},
		{"a suffix under the plain form", `alias -s t=z; alias t`, "testsh: alias: t: not found\n", 1},
		{"a regular under -s", `alias r=y; alias -s r`, "testsh: alias: r: not found\n", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := kindsRun(t, c.src)
			if out != c.want || st != c.status {
				t.Errorf("%s = %q (status %d), want %q (status %d)", c.src, out, st, c.want, c.status)
			}
		})
	}
}

// **`unalias -a` empties one table and `unalias -s -a` the other**, which is
// the strongest statement that they are two. A plain `unalias` cannot reach a
// suffix alias at all.
func TestUnaliasTakesOneNamespaceAtATime(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"-a leaves the suffix aliases",
			`alias r=y; alias -s t=z; unalias -a; echo "--plain--"; alias; echo "--s--"; alias -s`,
			"--plain--\n--s--\nt='z'\n",
		},
		{
			"-s -a leaves the others",
			`alias r=y; alias -s t=z; unalias -s -a; echo "--plain--"; alias; echo "--s--"; alias -s`,
			"--plain--\nr='y'\n--s--\n",
		},
		{
			"-s names one",
			`alias -s t=z; alias -s u=w; unalias -s t; echo "--s--"; alias -s`,
			"--s--\nu='w'\n",
		},
		{
			"the plain form cannot reach a suffix alias",
			`alias -s t=z; unalias t; echo "st=$?"; alias -s`,
			"testsh: unalias: t: not found\nst=1\nt='z'\n",
		},
		{
			"the plain form removes a global one",
			`alias -g G=x; unalias G; echo "st=$?"; alias`,
			"st=0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dg := Diagnostics{}
			out, _ := aliasRun(t, kindsOn, dg, c.src)
			if out != c.want {
				t.Errorf("%s = %q, want %q", c.src, out, c.want)
			}
		})
	}
}

// **Both letters at once is refused**, because one call cannot be about two
// namespaces — and nothing is defined by the call that was refused.
func TestBothKindsAtOnceIsRefused(t *testing.T) {
	out, st := kindsRun(t, `alias -g -s q=v; echo "st=$?"; alias; alias -s`)
	want := "testsh: alias: illegal combination of options\nst=1\n"
	if out != want || st != 0 {
		t.Errorf("alias -g -s = %q (status %d), want %q", out, st, want)
	}
}

// **A dialect without the kinds has neither letter**, which is the paired
// rule: the letters leave the accepted set with the axis, so the refusal is
// whatever that dialect says about an option nobody has.
func TestWithoutTheAxesTheLettersAreNotThere(t *testing.T) {
	dg := Diagnostics{BuiltinBadOption: "%[1]s: bad option: %[2]s"}
	for _, src := range []string{`alias -g G=x`, `alias -s t=z`, `unalias -s t`} {
		out, _ := aliasRun(t, nil, dg, src)
		if !strings.Contains(out, "bad option") {
			t.Errorf("%s with the axes off = %q, want a bad-option refusal", src, out)
		}
	}
}

// **The three tables are what the hooks answer from**, and each hook answers
// for its own kind alone. This is the seam the parser is handed, so a hook
// that answered from the wrong table would expand the wrong words everywhere
// at once.
func TestTheThreeHooksAnswerForTheirOwnKind(t *testing.T) {
	r := newTestRunner(t, &Runner{})
	r.SetAlias("r", "y")
	r.SetGlobalAlias("G", "x")
	r.SetSuffixAlias("t", "z")
	for _, c := range []struct {
		name    string
		look    func(string) (string, bool)
		present string
		absent  []string
	}{
		{"LookupAlias holds the regular and the global kind", r.LookupAlias, "r", []string{"t"}},
		{"LookupGlobalAlias holds the global kind alone", r.LookupGlobalAlias, "G", []string{"r", "t"}},
		{"LookupSuffixAlias holds the suffix kind alone", r.LookupSuffixAlias, "t", []string{"r", "G"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, ok := c.look(c.present); !ok {
				t.Errorf("%s not found", c.present)
			}
			for _, name := range c.absent {
				if _, ok := c.look(name); ok {
					t.Errorf("%s found, want it in another table", name)
				}
			}
		})
	}
	// LookupAlias answers for a global alias too, which is what makes a
	// global alias work in command position.
	if _, ok := r.LookupAlias("G"); !ok {
		t.Error("LookupAlias does not hold the global kind; command position would not expand one")
	}
}

// **The tables a subshell is given are copies**, for the reason the regular
// one already is: a suffix alias defined inside a subshell is not the
// parent's afterwards, and that is a *parse* difference rather than a value
// one.
func TestTheSuffixTableIsClonedForASubshell(t *testing.T) {
	out, _ := kindsRun(t, `alias -s t=z
(alias -s u=w)
alias -s`)
	want := "t='z'\n"
	if out != want {
		t.Errorf("a suffix alias defined in a subshell = %q, want %q", out, want)
	}
}
