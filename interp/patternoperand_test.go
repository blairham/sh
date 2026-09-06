// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A pattern operand is a word, and a word is expanded.
//
// It was not: everything but a parameter reached the matcher as the characters
// it was written as, so `${v#$(echo ab)}` stripped the five-character prefix
// `echo ab` — which `abcd` does not have — and answered `abcd`. Nothing said
// so. The value is asserted exactly here rather than "it did not fail",
// because "it did not fail" is what the bug did (#882).
//
// Measured across the panel: dash, bash 5.3, bash-as-sh, bash 3.2, ksh93 and
// zsh 5.9 all answer the right-hand column, in every position that has the
// operator at all — dash has neither `/` nor arrays.
func TestAPatternOperandIsExpanded(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"a prefix out of a command substitution", `v=abcd; printf "[%s]" "${v#$(echo ab)}"`, "[cd]"},
		{"a long prefix", `v=abcd; printf "[%s]" "${v##$(echo ab)}"`, "[cd]"},
		{"a suffix", `v=abcd; printf "[%s]" "${v%$(echo cd)}"`, "[ab]"},
		{"a long suffix", `v=abcd; printf "[%s]" "${v%%$(echo cd)}"`, "[ab]"},
		{"the backquoted spelling", "v=abcd; printf \"[%s]\" \"${v#`echo ab`}\"", "[cd]"},
		{"a replacement's pattern", `v=abcd; printf "[%s]" "${v/$(echo bc)/X}"`, "[aXd]"},
		{"every occurrence", `v=abcabc; printf "[%s]" "${v//$(echo b)/X}"`, "[aXcaXc]"},
		{"anchored at the front", `v=abcabc; printf "[%s]" "${v/#$(echo a)/X}"`, "[Xbcabc]"},
		{"anchored at the end", `v=abcabc; printf "[%s]" "${v/%$(echo c)/X}"`, "[abcabX]"},
		// The replacement side already expanded. It is here so that a change
		// to one side cannot quietly take the other with it.
		{"the replacement side", `v=abcd; printf "[%s]" "${v/b/$(echo Z)}"`, "[aZcd]"},
		{"an arithmetic expansion", `v=2bcd; printf "[%s]" "${v#$((1+1))}"`, "[bcd]"},
		{"an arithmetic expansion that does not match", `v=abcd; printf "[%s]" "${v#$((1+1))}"`, "[abcd]"},
		{"a quoted command substitution", `v=abcd; printf "[%s]" "${v#"$(echo ab)"}"`, "[cd]"},
		{"two substitutions in one operand", `v=abcd; printf "[%s]" "${v#$(echo a)$(echo b)}"`, "[cd]"},
		{"a parameter beside a substitution", `v=abcd; w=a; printf "[%s]" "${v#$w$(echo b)}"`, "[cd]"},
		// `$'\t'` is a tab everywhere in the panel but dash, which has no
		// such quoting at all. The lexer keeps both bytes so the source stays
		// recoverable, and the pattern operand never decoded them.
		{"a dollar-single escape", "v=$'\\tx'; printf \"[%s]\" \"${v#$'\\t'}\"", "[x]"},
		{"a case arm", `case ab in $(echo ab)) printf "[Y]";; *) printf "[N]";; esac`, "[Y]"},
		{"a case arm with an arithmetic expansion", `case 2ab in $((1+1))a*) printf "[Y]";; *) printf "[N]";; esac`, "[Y]"},
		{"a condition's right operand", `[[ ab == $(echo ab) ]] && printf "[Y]" || printf "[N]"`, "[Y]"},
		{"a condition, quoted arithmetic", `[[ 2ab == "$((1+1))"a* ]] && printf "[Y]" || printf "[N]"`, "[Y]"},
		{"a case change", `v=abcd; printf "[%s]" "${v^^$(echo b)}"`, "[aBcd]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, patternGrammar, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// patternGrammar turns on the constructs these operands need, by the
// construct's name rather than by a shell's.
func patternGrammar(d *syntax.Dialect) {
	d.ParamCaseChange = true
	d.ParamExpansionFlags = true
	d.ParamElementSelection = true
}

// The same word, applied to every element of an array: the substitution runs
// once and the pattern it produced is applied to each.
func TestAPatternOperandOverAnArrayIsExpandedOnce(t *testing.T) {
	// The count is kept in a file because a substitution runs in a subshell
	// and a variable it set would not come back — which is also why the
	// wrong answer here would be a quiet `n=x` either way if the marks were
	// counted in the shell.
	const src = `a=(abc bcd); printf "[%s]" "${a[@]#$(printf x >>marks; echo a)}"; ` +
		`printf "n=%s" "$(cat marks)"`
	out, st := runGrammar(t, src, patternGrammar, nil)
	// One mark, not two. A per-element implementation prints the same fields
	// and fires whatever side effects the word carries once per element;
	// measured, bash, ksh93 and zsh all run it exactly once.
	if want := "[bc][bcd]n=x"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// The result of an expansion is a pattern only where the dialect says so, and
// a pattern operand is not an exception to that.
//
// Measured: `v=abcdabcd; echo ${v##$(echo 'a*a')}` is `bcd` in dash, bash and
// ksh93 and `abcdabcd` in zsh — the same split as `p='a*a'; ${v##$p}`, which
// is one axis observed twice rather than two behaviors.
func TestAnExpandedPatternsMetacharactersFollowTheAxis(t *testing.T) {
	const src = `v=abcdabcd; printf "[%s]" "${v##$(echo 'a*a')}"`
	if out, st := runPatternAxis(t, Yes, src); out != "[bcd]" || st != 0 {
		t.Errorf("globbing the result: got %q (status %d), want %q at 0", out, st, "[bcd]")
	}
	if out, st := runPatternAxis(t, No, src); out != "[abcdabcd]" || st != 0 {
		t.Errorf("not globbing it: got %q (status %d), want %q at 0", out, st, "[abcdabcd]")
	}
}

// And where the axis is unanswered it is refused by name, rather than one
// shell's answer being picked.
func TestAnExpandedPatternWithAMetacharacterRefusesAnUnansweredAxis(t *testing.T) {
	out, st := runPatternAxis(t, Unspecified, `v=abcdabcd; printf "[%s]" "${v##$(echo 'a*a')}"`)
	want := "sh: globbing the result of an expansion: " +
		"the shells disagree here and no dialect was chosen\n"
	if !strings.HasPrefix(out, want) {
		t.Errorf("output = %q, want it to open with %q", out, want)
	}
	if st != 2 {
		t.Errorf("status %d, want the unanswered axis refused at 2", st)
	}
}

// The axis is asked only where the two answers differ. A result holding no
// metacharacter is the same text escaped or not, so a core that has chosen no
// dialect answers rather than refusing — which matters because this is the
// common spelling: every shell in the panel prints `cd` for both of these.
func TestAnExpandedPatternWithoutAMetacharacterAsksNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"out of a command substitution", `v=abcd; printf "[%s]" "${v#$(echo ab)}"`, "[cd]"},
		{"out of a parameter", `v=abcd; p=ab; printf "[%s]" "${v#$p}"`, "[cd]"},
		{"a case arm", `case ab in $(echo ab)) printf "[Y]";; *) printf "[N]";; esac`, "[Y]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runPatternAxis(t, Unspecified, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A process substitution in a pattern operand is not performed, and pinning
// that is the point rather than a gap left open.
//
// Measured, and the panel does not agree: in `${v#<(cmd)}` only bash runs the
// command; in a `case` arm bash and zsh both do and ksh93 and dash cannot
// parse it; in `[[ ]]` bash runs it where zsh refuses the word outright. There
// is no intersection, so the core does not invent one — and the test asserts
// the command did not run, which is the half a change here would break
// silently. The divergence itself is #902.
func TestAProcessSubstitutionInAPatternIsNotPerformed(t *testing.T) {
	// The subject is the value coming back whole, and the evidence that
	// nothing ran is the scratch directory: a substitution that is performed
	// makes an `sh-procsub…` directory under the Runner's TMPDIR to hold its
	// pipe. Asserting on a variable the command sets would prove nothing —
	// the command runs in a child, so an assignment it makes never comes
	// back either way.
	//
	// The arm is where the word carries a process substitution at all: inside
	// `${…}` the `<(` is ordinary text and never becomes one, so a test
	// written there would assert about a branch it does not reach.
	tmp := t.TempDir()
	out, st := runGrammar(t, `case abc in <(:)) printf "[hit]";; *) printf "[miss]";; esac`,
		patternGrammar, func(r *Runner) { r.Env = append(withoutTMPDIR(r.Env), "TMPDIR="+tmp) })
	// A miss either way — bash and zsh match `abc` against the path they made
	// and do not match it either — so the outcome is the panel's and the
	// question this pins is the one beside it.
	if want := "[miss]"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
	made, err := filepath.Glob(filepath.Join(tmp, "sh-procsub*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(made) != 0 {
		t.Errorf("a process substitution ran: %v", made)
	}
}

// runPatternAxis runs src with one answer for GlobExpansionResults and the
// core's answers for everything else, so the axis under test is the only one
// that can decide anything.
func runPatternAxis(t *testing.T, glob Answer, src string) (out string, status int) {
	t.Helper()
	return runGrammar(t, src, patternGrammar, func(r *Runner) {
		sem := CoreSemantics()
		sem.GlobExpansionResults = glob
		r.Semantics = &sem
	})
}

// withoutTMPDIR drops the scratch directory the test helper supplies, so a
// test naming its own is the one that answers. Appending a second entry is
// not enough: a Runner reads the first it finds, which is the helper's.
func withoutTMPDIR(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, "TMPDIR=") {
			out = append(out, kv)
		}
	}
	return out
}
