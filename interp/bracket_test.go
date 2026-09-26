// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// bracketSem answers the bracket axis by name; everything else is the
// permissive base. Which preset gives which answer is asserted in the
// dialect packages, not here.
func bracketSem(p BracketPolicy) Semantics {
	s := permissive()
	s.UnterminatedBracket = p
	return s
}

// TestUnterminatedBracketHasThreeAnswers is the axis that does not fit Answer.
// `[` is the name of the test builtin, so this is load-bearing rather than
// exotic.
func TestUnterminatedBracketHasThreeAnswers(t *testing.T) {
	const src = `case "[" in [) echo hit;; *) echo miss;; esac`
	for _, tc := range []struct {
		policy BracketPolicy
		want   string
	}{
		{BracketLiteral, "hit\n"},
		{BracketNoMatch, "miss\n"},
	} {
		if got, _ := run(t, src, withSem(bracketSem(tc.policy))); got != tc.want {
			t.Errorf("%v: got %q, want %q", tc.policy, got, tc.want)
		}
	}
	// BracketBadPattern rejects the pattern and abandons the script — at the
	// dialect's own **fatal** status, which is the same number the pattern
	// gives against the filesystem. This line used to want 0, and the
	// measurement behind it read a *subshell's* `$?` rather than the script's
	// own exit: re-measured 2026-09-18 from a script file, `case '[a' in ([)
	// echo one;; (*) echo two;; esac` ends real zsh at 1, and the `[[ ]]`
	// spelling of the same pattern ends it at 2 (#3398).
	//
	// Both answers to FatalErrorStatusIsOne, because the number is that
	// axis's and not this one's: a test that pinned only the shell this was
	// found on would read as a constant.
	for _, tc := range []struct {
		fatalIsOne Answer
		want       int
	}{{Yes, 1}, {No, 2}} {
		sem := bracketSem(BracketBadPattern)
		sem.FatalErrorStatusIsOne = tc.fatalIsOne
		out, st := run(t, src+`; echo after`, withSem(sem))
		if !strings.Contains(out, "bad pattern: [") {
			t.Errorf("bad-pattern policy: got %q", out)
		}
		if strings.Contains(out, "after") || strings.Contains(out, "hit") || strings.Contains(out, "miss") {
			t.Errorf("bad-pattern policy: the script should stop, got %q", out)
		}
		if st != tc.want {
			t.Errorf("bad-pattern policy: status = %d, want %d", st, tc.want)
		}
	}
	// The core has no answer and says so.
	if _, st := run(t, src, withSem(CoreSemantics())); st != 2 {
		t.Errorf("the core should refuse, status %d", st)
	}
}

// TestBracketPolicyIsAskedOnlyWhenItApplies keeps ordinary patterns usable in
// the core, the same way the caret axis does.
func TestBracketPolicyIsAskedOnlyWhenItApplies(t *testing.T) {
	if got, st := run(t, `case b in [abc]) echo in;; *) echo out;; esac`, withSem(CoreSemantics())); got != "in\n" || st != 0 {
		t.Errorf("a closed class needs no answer: %q status %d", got, st)
	}
	if got, _ := run(t, `case a in a) echo plain;; esac`, withSem(CoreSemantics())); got != "plain\n" {
		t.Errorf("a pattern with no bracket at all: %q", got)
	}
}

// TestTestBuiltinSurvivesTheBracketPolicy is why this axis matters: `[` is a
// command name, and a policy applied to the filesystem too eagerly stops it
// running.
func TestTestBuiltinSurvivesTheBracketPolicy(t *testing.T) {
	for _, policy := range []BracketPolicy{BracketLiteral, BracketNoMatch, BracketBadPattern} {
		sem := bracketSem(policy)
		if got, _ := run(t, `[ a = a ] && echo yes`, withSem(sem)); got != "yes\n" {
			t.Errorf("%v: got %q, want %q", policy, got, "yes\n")
		}
		if got, _ := run(t, `echo [`, withSem(sem)); got != "[\n" {
			t.Errorf("%v: a lone [ is literal against the filesystem, got %q", policy, got)
		}
	}
	// The bad-pattern policy rejects one with anything else in the field,
	// and that is the whole field rather than where the bracket sits in it.
	for _, src := range []string{`echo [a`, `echo a[`} {
		out, st := run(t, src, withSem(bracketSem(BracketBadPattern)))
		if !strings.Contains(out, "bad pattern") {
			t.Errorf("bad-pattern %s: got %q", src, out)
		}
		if st != 1 {
			t.Errorf("bad-pattern %s: status = %d, want 1", src, st)
		}
	}
}

// TestEqualsExpansionRunsBeforeGlobbing is measured rather than assumed:
// `echo [[a == a]]` reports the `==` and never reaches the `[[a`. Both axes
// are named here, because the ordering only shows where both are on.
func TestEqualsExpansionRunsBeforeGlobbing(t *testing.T) {
	sem := bracketSem(BracketBadPattern)
	sem.EqualsExpansion = Yes
	out, _ := run(t, `echo [[a == a]]`, withSem(sem))
	if strings.Contains(out, "bad pattern") {
		t.Errorf("globbing ran first: %q", out)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("got %q", out)
	}
}

// TestABracketFromAValueIsNotAPatternWhereExpansionsAreNotGlobbed is #1386,
// and it needs both axes named because the bug lives where they meet.
//
// The bracket axis alone says an unterminated `[` is a bad pattern. The other
// axis says whether the result of an expansion is a pattern at all. Where it
// says no, a value carrying `a[1m` is text and every shell prints it — but
// the refusal fired first, before anything asked, so the unquoted spelling
// abandoned the script. `ESC [` opens every ANSI escape sequence, which is
// what makes this the difference between a shell that can hold a color in a
// variable and one that cannot.
func TestABracketFromAValueIsNotAPatternWhereExpansionsAreNotGlobbed(t *testing.T) {
	sem := bracketSem(BracketBadPattern)
	sem.GlobExpansionResults = No

	// The value arriving five ways. All five were fatal, and a fix that
	// escaped only the scalar variable would leave the last three.
	for _, src := range []string{
		`m="a[1m"; echo $m`,
		`m="a[1m"; echo ${m}`,
		`m="a[1m"; x=$m; echo $x`,
		`c=$(printf 'a[1m'); echo $c`,
		`set -- "a[1m"; echo $1`,
	} {
		out, st := run(t, src, withSem(sem))
		if out != "a[1m\n" || st != 0 {
			t.Errorf("%s: got %q status %d, want %q status 0", src, out, st, "a[1m\n")
		}
	}
	// Two of them in one field, because the field the matcher sees is the
	// join and an escape applied per span has to survive it.
	if out, st := run(t, `m="a[1m"; echo $m$m`, withSem(sem)); out != "a[1ma[1m\n" || st != 0 {
		t.Errorf("joined: got %q status %d", out, st)
	}
	// Quoted, which was already right and is the axis held fixed.
	if out, _ := run(t, `m="a[1m"; echo "$m"`, withSem(sem)); out != "a[1m\n" {
		t.Errorf("quoted: got %q", out)
	}

	// The control that must keep failing. A bracket *written in the source*
	// is a pattern whatever this axis says, so it is still refused — and by
	// the time the field reaches the matcher the escaping is the only thing
	// that says which of the two it was. A fix in glob could only have been
	// one that lost this.
	out, st := run(t, `echo a[1m`, withSem(sem))
	if !strings.Contains(out, "bad pattern") || st != 1 {
		t.Errorf("a literal bracket must still be refused: got %q status %d", out, st)
	}

	// And the composition, from the other side: where the dialect *does*
	// glob the result of an expansion, the value really is a pattern and
	// refusing it is right. This is the row that fails if the fix stops
	// asking the axis and escapes unconditionally.
	globbed := bracketSem(BracketBadPattern)
	globbed.GlobExpansionResults = Yes
	out, st = run(t, `m="a[1m"; echo $m`, withSem(globbed))
	if !strings.Contains(out, "bad pattern") || st != 1 {
		t.Errorf("with expansion results globbed the refusal is correct: got %q status %d", out, st)
	}

	// A *terminated* bracket from a value is a legal pattern that matches
	// nothing, so it is passed through rather than refused. It is the row a
	// looser fix would have moved, and it moves under neither axis.
	for _, s := range []Semantics{sem, globbed} {
		if out, st := run(t, `t="a[b]"; echo $t`, withSem(s)); out != "a[b]\n" || st != 0 {
			t.Errorf("a terminated bracket from a value: got %q status %d", out, st)
		}
	}
}

// TestAParameterExpansionsOperandAsksTheBracketAxis is #4646: the surface had
// no answer of its own and gave one anyway.
//
// `${v#pat}` and its family compile their operand as a pattern, and the axis
// says what text an unterminated bracket is. That was pinned to BracketLiteral
// on this path, on the reading that every shell reading it serves is literal
// there — and the table on Semantics.UnterminatedBracketAfterASubExpression,
// twenty lines above the axis itself, is a prefix trim splitting three ways.
// So two of the three answers were unreachable here: the literal columns were
// right by accident, the no-match column stripped a prefix it should not have
// matched, and the bad-pattern column **reported success with a value it
// should have refused to compute**.
//
// The rule with one noun in it: the axis decides, not the surface. Every row
// below is the same text `[a` on the same operators, and only the axis moves.
func TestAParameterExpansionsOperandAsksTheBracketAxis(t *testing.T) {
	for _, op := range []struct{ name, src, literal, noMatch string }{
		{"a short prefix trim", `v='[abc'; echo "${v#[a}"`, "bc\n", "[abc\n"},
		{"a long prefix trim", `v='[abc'; echo "${v##[a}"`, "bc\n", "[abc\n"},
		{"a short suffix trim", `v='bc[a'; echo "${v%[a}"`, "bc\n", "bc[a\n"},
		{"a long suffix trim", `v='bc[a'; echo "${v%%[a}"`, "bc\n", "bc[a\n"},
		{"a substitution", `v='[abc'; echo "${v/[a/x}"`, "xbc\n", "[abc\n"},
		{"a global substitution", `v='[abc'; echo "${v//[a/x}"`, "xbc\n", "[abc\n"},
	} {
		t.Run(op.name, func(t *testing.T) {
			if got, st := run(t, op.src, withSem(bracketSem(BracketLiteral))); got != op.literal || st != 0 {
				t.Errorf("literal: got %q status %d, want %q", got, st, op.literal)
			}
			if got, st := run(t, op.src, withSem(bracketSem(BracketNoMatch))); got != op.noMatch || st != 0 {
				t.Errorf("no match: got %q status %d, want %q", got, st, op.noMatch)
			}
			out, st := run(t, op.src+`; echo after`, withSem(bracketSem(BracketBadPattern)))
			if !strings.Contains(out, "bad pattern: [a") {
				t.Errorf("bad pattern: got %q", out)
			}
			if strings.Contains(out, "after") {
				t.Errorf("bad pattern: the script should stop, got %q", out)
			}
			if st != 1 {
				t.Errorf("bad pattern: status = %d, want 1", st)
			}
		})
	}
	// The core has no answer and says so — once, however many times the
	// expansion consults the axis on its way through.
	out, st := run(t, `v='[abc'; echo "${v#[a}"`, withSem(CoreSemantics()))
	if st != 2 {
		t.Errorf("the core should refuse, status %d", st)
	}
	if n := strings.Count(out, "unterminated bracket"); n != 1 {
		t.Errorf("the axis should be resolved once, reported %d times: %q", n, out)
	}
}

// TestAnOperandIsCompiledBeforeItIsMatched is the discriminating pair for the
// same rule, and it is the one a refusal raised from inside the matcher cannot
// produce.
//
// `x[a` is ruled out by its first character, and the span walk skips candidate
// pieces the pattern's edge literals could not fill — so on this subject the
// matcher is never asked and a match-time refusal would stay silent. Whether a
// pattern compiles is not a question about the value, so neither is the
// refusal.
func TestAnOperandIsCompiledBeforeItIsMatched(t *testing.T) {
	sem := bracketSem(BracketBadPattern)
	for _, src := range []string{
		`v=zzz; echo "${v#[a}"`,
		`v=zzz; echo "${v#x[a}"`,
		`v=zzz; echo "${v%a[x}"`,
		`v=zzz; echo "${v//x[a/q}"`,
	} {
		out, st := run(t, src+`; echo after`, withSem(sem))
		if !strings.Contains(out, "bad pattern") || strings.Contains(out, "after") || st != 1 {
			t.Errorf("%s: got %q status %d, want a refusal at 1", src, out, st)
		}
	}
	// Held against a pattern that compiles and misses, which leaves the same
	// value behind at status 0. Reading the value alone cannot tell the two
	// apart, which is why every row above reads the status.
	if got, st := run(t, `v=zzz; echo "${v#q}"`, withSem(sem)); got != "zzz\n" || st != 0 {
		t.Errorf("a pattern that misses: got %q status %d", got, st)
	}
	// And against a bracket that closes, which is a pattern under every
	// answer the axis has.
	for _, p := range []BracketPolicy{BracketLiteral, BracketNoMatch, BracketBadPattern} {
		if got, st := run(t, `v='[abc'; echo "${v#[ab]}"`, withSem(bracketSem(p))); got != "[abc\n" || st != 0 {
			t.Errorf("%v: a closed bracket: got %q status %d", p, got, st)
		}
	}
}
