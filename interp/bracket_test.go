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
	// BracketBadPattern rejects the pattern and abandons the script — with
	// status 0, where the same pattern against the filesystem gives 1. Both
	// are measured; neither is guessable from the other.
	out, st := run(t, src+`; echo after`, withSem(bracketSem(BracketBadPattern)))
	if !strings.Contains(out, "bad pattern: [") {
		t.Errorf("bad-pattern policy: got %q", out)
	}
	if strings.Contains(out, "after") || strings.Contains(out, "hit") || strings.Contains(out, "miss") {
		t.Errorf("bad-pattern policy: the script should stop, got %q", out)
	}
	if st != 0 {
		t.Errorf("bad-pattern policy: status = %d, want 0", st)
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
