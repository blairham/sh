// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// tildeFixture builds the tree every row below is measured against: a home
// directory holding `zz` and `qq`, and a `d` directory holding three files a
// pattern matches and one it does not.
//
// Under t.TempDir(), never a real home: this construct is about filename
// generation, so the rows only mean anything against a directory the test
// owns.
func tildeFixture(t *testing.T) (home, dir string) {
	t.Helper()
	root := t.TempDir()
	home = filepath.Join(root, "home")
	dir = filepath.Join(root, "d")
	for _, d := range []string{home, dir, filepath.Join(home, "zz"), filepath.Join(home, "qq")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"inner.sh", "inner2.sh", "inner3.sh", "other.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return home, dir
}

// tildeGrammar is the grammar this construct needs, named by the constructs
// rather than by a shell: the flag itself, a subscript so the shape zi.zsh
// uses can be written, and the parenthesized group the flag sits behind.
func tildeGrammar(d *syntax.Dialect) {
	d.ParamTildeFlag = true
	d.ArraySubscript = true
	d.ParamExpansionFlags = true
}

// runTilde runs src with the grammar the flag needs and this shell's two
// answers that the flag exists to override: an unquoted expansion's result is
// not a pattern here, and a pattern matching nothing is an error.
func runTilde(t *testing.T, home, dir, src string) (string, int) {
	t.Helper()
	d := syntax.Core()
	tildeGrammar(&d)
	return runGrammar(t, src, tildeGrammar, func(r *Runner) {
		r.Dialect = &d
		r.Dir = dir
		sem := *r.Semantics
		sem.GlobExpansionResults = No
		sem.GlobNoMatchIsError = Yes
		sem.SplitParamExpansion = No
		r.Semantics = &sem
		r.Env = append(r.Env, "HOME="+home)
	})
}

// The two halves of the flag, and the three behaviors that carry it.
//
// Every row is a measurement on zsh 5.9.2, 2026-09-06, against exactly the
// tree tildeFixture builds. The values are asserted rather than the absence
// of a diagnostic: a fix that silenced the error without producing the right
// value would be worse than the refusal it replaced, because eighteen of
// these in a row is how a real plugin manager builds its paths.
func TestTheTildeFlagExpandsATildeAndAPattern(t *testing.T) {
	home, dir := tildeFixture(t)
	for _, tc := range []struct{ name, src, want string }{
		{
			"without the flag the value is the value",
			`g="$D/inn*.sh"; printf "[%s]" ${g}`,
			`[DIR/inn*.sh]`,
		},
		{
			"with it the pattern is matched, one word per match",
			`g="$D/inn*.sh"; printf "[%s]" ${~g}`,
			`[DIR/inner.sh][DIR/inner2.sh][DIR/inner3.sh]`,
		},
		{
			"quoting suppresses it",
			`g="$D/inn*.sh"; printf "[%s]" "${~g}"`,
			`[DIR/inn*.sh]`,
		},
		{
			"doubled turns it back off",
			`g="$D/inn*.sh"; printf "[%s]" ${~~g}`,
			`[DIR/inn*.sh]`,
		},
		{
			"and tripled on again — it is parity",
			`g="$D/inn*.sh"; printf "[%s]" ${~~~g}`,
			`[DIR/inner.sh][DIR/inner2.sh][DIR/inner3.sh]`,
		},
		{
			"a tilde in the value expands",
			`t='~/zz'; printf "[%s]" ${~t}`,
			`[HOME/zz]`,
		},
		{
			"and the rest of the word joins what it came to",
			`t='~/zz'; printf "[%s]" ${~t}/sub`,
			`[HOME/zz/sub]`,
		},
		{
			"quoting suppresses the tilde half too",
			`t='~/zz'; printf "[%s]" "${~t}"`,
			`[~/zz]`,
		},
		{
			"doubled turns the tilde half off as well",
			`t='~/zz'; printf "[%s]" ${~~t}`,
			`[~/zz]`,
		},
		{
			"a subscript is no obstacle — the shape zi.zsh uses eighteen times",
			`typeset -A A; A[h]='~/zz'; printf "[%s]" ${~A[h]}`,
			`[HOME/zz]`,
		},
		{
			"an already absolute value passes through unchanged",
			`typeset -A Z; Z[H]="$HOME/.zi"; Z[H]=${~Z[H]}; printf "[%s]" "${Z[H]}"`,
			`[HOME/.zi]`,
		},
		{
			"the tilde expands before the pattern is matched",
			`v='~/*'; printf "[%s]" ${~v}`,
			`[HOME/qq][HOME/zz]`,
		},
		{
			"and only at the head of the value",
			`v='a:~/zz'; printf "[%s]" ${~v}`,
			`[a:~/zz]`,
		},
		{
			"the operator runs first and the flag marks what it produced",
			`v="X$D/inn*.sh"; printf "[%s]" ${~v#X}`,
			`[DIR/inner.sh][DIR/inner2.sh][DIR/inner3.sh]`,
		},
		{
			"including an operator that substitutes a word instead",
			`printf "[%s]" ${~nosuch:-"$D/inn*.sh"}`,
			`[DIR/inner.sh][DIR/inner2.sh][DIR/inner3.sh]`,
		},
		{
			// The tilde half reaches that operand too, and it did not:
			// the operand's fields never pass through the split path
			// where tildeFlagElements is applied, so this was the text
			// it was written as (#1500).
			"and the tilde half reaches the operand as well",
			`printf "[%s]" ${~nosuch:-"~/zz"}`,
			`[HOME/zz]`,
		},
		{
			// Where a written tilde would have expanded, which is the
			// head of a word — the same limit the value form has.
			"only at the head, there as everywhere",
			`printf "[%s]" X${~nosuch:-"~/zz"}`,
			`[X~/zz]`,
		},
		{
			// The off parity switches off the flag, not the operand's own
			// metacharacters: those are written rather than substituted,
			// so they were never the flag's to reach. Measured on zsh
			// 5.9.2 — `${~~u:-X[a-b]y}` matches and `${~~u:-"X[a-b]y"}`
			// does not.
			"the off parity leaves a written pattern written",
			`printf "[%s]" ${~~nosuch:-$D/inn*.sh}`,
			`[DIR/inner.sh][DIR/inner2.sh][DIR/inner3.sh]`,
		},
		{
			"and leaves a quoted operand quoted",
			`printf "[%s]" ${~~nosuch:-"$D/inn*.sh"}`,
			`[DIR/inn*.sh]`,
		},
		{
			"a length leaves nothing for the flag to do",
			`v='~/zz'; printf "[%s]" ${~#v}`,
			`[4]`,
		},
		{
			"a flag group may precede it",
			`v='~/ZZ'; printf "[%s]" ${(L)~v}`,
			`[HOME/zz]`,
		},
		{
			"an empty name is legal once a tilde is written",
			`printf "[%s]" "${~}"`,
			`[]`,
		},
		{
			"the pattern half reaches a list, one element at a time",
			`a=("$D/inn*.sh"); printf "[%s]" ${~a[@]}`,
			`[DIR/inner.sh][DIR/inner2.sh][DIR/inner3.sh]`,
		},
		{
			"and without a tilde the same list is the dialect's no",
			`a=("$D/inn*.sh"); printf "[%s]" ${a[@]}`,
			`[DIR/inn*.sh]`,
		},
		{
			"a flag group and the pattern half together",
			`g='INN*.SH'; printf "[%s]" ${(L)~g}`,
			`[inner.sh][inner2.sh][inner3.sh]`,
		},
		{
			"the same group without the tilde matches nothing",
			`g='INN*.SH'; printf "[%s]" ${(L)g}`,
			`[inn*.sh]`,
		},
		{
			"a flag group whose result is a list: the tilde half",
			`a=('~/zz' '~/qq'); printf "[%s]" ${(o)~a}`,
			`[HOME/qq][HOME/zz]`,
		},
		{
			"and the pattern half",
			`a=('inn*.sh' 'oth*.txt'); printf "[%s]" ${(o)~a}`,
			`[inner.sh][inner2.sh][inner3.sh][other.txt]`,
		},
		{
			"and neither without the tilde",
			`a=('inn*.sh' 'oth*.txt'); printf "[%s]" ${(o)a}`,
			`[inn*.sh][oth*.txt]`,
		},
		{
			"an assignment is a context that splits nothing, and the head rule holds there too",
			`t='~/zz'; q=${~t}; r=x${~t}; printf "[%s]" "$q" "$r"`,
			`[HOME/zz][x~/zz]`,
		},
		{
			"the head of the word is what the tilde half asks about",
			`t='~/zz'; printf "[%s]" x${~t}`,
			`[x~/zz]`,
		},
		{
			"and an empty span in front leaves the head where it was",
			`a=''; t='~/zz'; printf "[%s]" ${a}${~t}`,
			`[HOME/zz]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runTilde(t, home, dir, `D=`+dir+"\n"+tc.src)
			got := strings.ReplaceAll(strings.ReplaceAll(out, dir, "DIR"), home, "HOME")
			if got != tc.want || st != 0 {
				t.Errorf("%s\n got %q (status %d)\nwant %q at 0", tc.src, got, st, tc.want)
			}
		})
	}
}

// The word form splits into as many words as the pattern matched, which is the
// half a joined string would hide.
func TestTheTildeFlagSplitsIntoAsManyWordsAsItMatched(t *testing.T) {
	home, dir := tildeFixture(t)
	out, st := runTilde(t, home, dir,
		`D=`+dir+"\ng=\"$D/inn*.sh\"\nw=( ${~g} )\nprintf \"%s\" \"${#w[@]}\"")
	if out != "3" || st != 0 {
		t.Errorf("count = %q (status %d), want 3", out, st)
	}
	// And the count is three because three *arguments* arrive, not because a
	// string was split later: a function counting `$#` sees the same three.
	out, st = runTilde(t, home, dir,
		`D=`+dir+"\nf() { printf \"%s\" \"$#\"; }\ng=\"$D/inn*.sh\"\nf ${~g}")
	if out != "3" || st != 0 {
		t.Errorf("$# = %q (status %d), want 3", out, st)
	}
	// One word without the flag, which is what makes the row above a fact
	// about the flag.
	out, st = runTilde(t, home, dir,
		`D=`+dir+"\nf() { printf \"%s\" \"$#\"; }\ng=\"$D/inn*.sh\"\nf ${g}")
	if out != "1" || st != 0 {
		t.Errorf("$# without the flag = %q (status %d), want 1", out, st)
	}
}

// A list expands elementwise, and each element after the first is the head of
// its own word however the expansion was written.
func TestTheTildeFlagReachesEveryElementOfAList(t *testing.T) {
	home, dir := tildeFixture(t)
	for _, tc := range []struct{ src, want string }{
		{`set -- '~/zz' '~/qq'; printf "[%s]" ${~@}`, `[HOME/zz][HOME/qq]`},
		{`a=('~/zz' '~/qq'); printf "[%s]" ${~a[@]}`, `[HOME/zz][HOME/qq]`},
		// The prefix takes the first element out of head position and leaves
		// the second in it, which is measured and is the reason the rule is
		// about a field's head rather than a span's.
		{`set -- '~/zz' '~/qq'; printf "[%s]" X${~@}`, `[X~/zz][HOME/qq]`},
		// Quoted, the whole construct is suppressed.
		{`set -- '~/zz' '~/qq'; printf "[%s]" "${~@}"`, `[~/zz][~/qq]`},
	} {
		out, st := runTilde(t, home, dir, tc.src)
		got := strings.ReplaceAll(out, home, "HOME")
		if got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, got, st, tc.want)
		}
	}
}

// Marking the value a pattern is the same question GlobExpansionResults
// answers, so the flag reaches every place that answer is read — the
// conditional, a `case` subject and a pattern operand, not only the word that
// goes to the filesystem.
func TestTheTildeFlagMarksAPatternOperandToo(t *testing.T) {
	home, dir := tildeFixture(t)
	for _, tc := range []struct{ src, want string }{
		{`p='a*'; case abc in ${~p}) printf yes;; *) printf no;; esac`, "yes"},
		{`p='a*'; case abc in ${p}) printf yes;; *) printf no;; esac`, "no"},
		{`v=abc; p='a*'; printf "[%s]" ${v#${~p}}`, "[bc]"},
		{`v=abc; p='a*'; printf "[%s]" ${v#${p}}`, "[abc]"},
		{`v=abc; p='a*'; printf "[%s]" ${v#${~~p}}`, "[abc]"},
	} {
		out, st := runTilde(t, home, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// The run-time option still wins: it is about whether the filesystem pass
// happens at all, not about what the word is.
func TestTheTildeFlagIsStillSubjectToTheGlobOptions(t *testing.T) {
	home, dir := tildeFixture(t)
	out, st := runTilde(t, home, dir, `D=`+dir+"\nset -f\ng=\"$D/inn*.sh\"\nprintf \"[%s]\" ${~g}")
	if want := "[DIR/inn*.sh]"; strings.ReplaceAll(out, dir, "DIR") != want || st != 0 {
		t.Errorf("with noglob = %q (status %d), want %q", out, st, want)
	}
	// A pattern matching nothing is the error GlobNoMatchIsError makes it,
	// which is the same treatment a written pattern gets.
	out, st = runTilde(t, home, dir, `D=`+dir+"\ng=\"$D/zznope*\"\nprintf \"[%s]\" ${~g}")
	if !strings.Contains(out, "no matches found") || st == 0 {
		t.Errorf("a miss = %q (status %d), want it reported and a failure", out, st)
	}
}

// Without the grammar flag the expansion is unreadable, and it is reported
// when it is *reached* — the deferred half of the bad-substitution split.
// A branch never taken diagnoses nothing, which is measured.
func TestWithoutTheGrammarFlagTheTildeIsABadSubstitution(t *testing.T) {
	out, st := runGrammar(t, `g=x; echo ${~g}`, nil, nil)
	if !strings.Contains(out, "bad substitution") || st == 0 {
		t.Errorf("got %q (status %d), want a bad substitution and a failure", out, st)
	}
	out, st = runGrammar(t, "if false; then : ${~g}; fi\necho reached", nil, nil)
	if out != "reached\n" || st != 0 {
		t.Errorf("in a branch never taken: %q (status %d), want a clean run", out, st)
	}
}

// A dialect that does not have the flag is not changed by its arrival: the
// same expansion, the same answer.
func TestTheFlagChangesNothingWhereItIsOff(t *testing.T) {
	home, dir := tildeFixture(t)
	// The pattern half without any tilde written is the dialect's answer and
	// nothing else, on both paths — the scalar one and the list one.
	for _, tc := range []struct {
		src  string
		glob Answer
		want string
	}{
		{`D=` + dir + "\ng=\"$D/inn*.sh\"; printf \"[%s]\" ${g}", No, `[DIR/inn*.sh]`},
		{`D=` + dir + "\ng=\"$D/inn*.sh\"; printf \"[%s]\" ${g}", Yes, `[DIR/inner.sh][DIR/inner2.sh][DIR/inner3.sh]`},
		{`D=` + dir + "\na=(\"$D/inn*.sh\"); printf \"[%s]\" ${a[@]}", No, `[DIR/inn*.sh]`},
		{`D=` + dir + "\na=(\"$D/inn*.sh\"); printf \"[%s]\" ${a[@]}", Yes, `[DIR/inner.sh][DIR/inner2.sh][DIR/inner3.sh]`},
	} {
		d := syntax.Core()
		tildeGrammar(&d)
		out, st := runGrammar(t, tc.src, tildeGrammar, func(r *Runner) {
			r.Dialect = &d
			r.Dir = dir
			sem := *r.Semantics
			sem.GlobExpansionResults = tc.glob
			sem.SplitParamExpansion = No
			r.Semantics = &sem
			r.Env = append(r.Env, "HOME="+home)
		})
		if got := strings.ReplaceAll(out, dir, "DIR"); got != tc.want || st != 0 {
			t.Errorf("%s with glob=%v = %q (status %d), want %q", tc.src, tc.glob, got, st, tc.want)
		}
	}
}

// The contexts that are not an ordinary word, each of which decides the head
// question for itself. Measured 2026-09-06 on zsh 5.9.2.
func TestTheTildeFlagInTheContextsThatAreNotAWord(t *testing.T) {
	home, dir := tildeFixture(t)

	// A redirection target: the tilde expands at the head of it, so the file
	// lands in the home directory rather than beside a literal `~`.
	out, st := runTilde(t, home, dir, "t='~/out'\nprintf hi > ${~t}\ncat \"$HOME/out\"")
	if out != "hi" || st != 0 {
		t.Errorf("a target at the head = %q (status %d), want the file in HOME", out, st)
	}
	// And with something in front of it the tilde is not at a head, so the
	// path names a directory that is not there — which is the observable
	// half of the rule rather than a quieter identical answer.
	out, st = runTilde(t, home, dir, "t='~/out'\nprintf hi > x${~t}")
	if !strings.Contains(out, "x~/out") || st == 0 {
		t.Errorf("a prefixed target = %q (status %d), want `x~/out` named and a failure", out, st)
	}

	// A here-document body is input rather than a word: neither half of the
	// flag applies there.
	out, st = runTilde(t, home, dir, "t='~/zz'\ncat <<XX\n${~t}\nXX")
	if out != "~/zz\n" || st != 0 {
		t.Errorf("a here-document body = %q (status %d), want the value unchanged", out, st)
	}

	// A nested expansion's inner is the head of its own word.
	d := syntax.Core()
	tildeGrammar(&d)
	d.NestedParamExpansion = true
	out, st = runGrammar(t, "t='~/zz'\nprintf \"[%s]\" ${${~t}}", func(g *syntax.Dialect) {
		tildeGrammar(g)
		g.NestedParamExpansion = true
	}, func(r *Runner) {
		r.Dialect = &d
		r.Dir = dir
		sem := *r.Semantics
		sem.GlobExpansionResults = No
		sem.SplitParamExpansion = No
		r.Semantics = &sem
		r.Env = append(r.Env, "HOME="+home)
	})
	if got := strings.ReplaceAll(out, home, "HOME"); got != "[HOME/zz]" || st != 0 {
		t.Errorf("a nested inner = %q (status %d), want [HOME/zz]", got, st)
	}
}
