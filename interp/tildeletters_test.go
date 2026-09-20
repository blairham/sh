// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"testing"
)

// The three classes a `~(…)` letter can fall into, pinned against each other.
//
// The tables in tildemodifier.go are the measurement; this is the check that
// the reader agrees with them, and that nothing lands in two classes at once.
// A letter that is honored but missing from the accepted set would be one this
// shell answers and ksh93 does not, which is a construct invented here; one
// that is accepted and neither honored nor refused would be silently dropped,
// which is the failure the refusal exists to prevent.
func TestEveryTildeLetterHasExactlyOneReading(t *testing.T) {
	for i := 0; i < len(honoredTildeLetters); i++ {
		c := honoredTildeLetters[i]
		if strings.IndexByte(kshTildeLetters, c) < 0 {
			t.Errorf("~(%c) is honored here and is not a letter ksh93 takes", c)
		}
	}
	for _, c := range []byte(kshTildeLetters) {
		m, unhonored := readTildeModifier(string(c))
		honored := strings.IndexByte(honoredTildeLetters, c) >= 0
		switch {
		case honored && unhonored != 0:
			t.Errorf("~(%c) is in the honored set and was refused", c)
		case !honored && unhonored != c:
			t.Errorf("~(%c) is not honored and was taken as %#v", c, m)
		case m.flavor == tildeNever:
			t.Errorf("~(%c) is a letter ksh93 takes and read as never-matching", c)
		}
	}
	// And a letter that shell does not have reads as the pattern that cannot
	// match, silently — measured, `[[ abc == ~(Z)abc ]]` and
	// `[[ abc == ~(Z)* ]]` are both status 1 with an empty standard error.
	for _, c := range []byte("ZRCDHIJQTWYbcdefhjknoqtuvwyz") {
		if strings.IndexByte(kshTildeLetters, c) >= 0 {
			t.Fatalf("~(%c) is in the accepted set; the probe list is wrong", c)
		}
		m, unhonored := readTildeModifier(string(c))
		if unhonored != 0 || m.flavor != tildeNever {
			t.Errorf("~(%c): read as %#v with unhonored %q, want a never-matching pattern", c, m, unhonored)
		}
	}
}

// The letters that are honored, and what each one is for. One row apiece, so a
// reading that quietly changed — `L` becoming an anchor, say, or `i` folding
// the wrong side — fails here rather than in a shell probe that happens to
// cover it.
func TestTheHonoredTildeLettersRead(t *testing.T) {
	for _, c := range []struct {
		body string
		want tildeModifier
	}{
		{"", tildeModifier{}},
		{"K", tildeModifier{flavor: tildeGlob}},
		{"E", tildeModifier{flavor: tildeERE}},
		{"F", tildeModifier{flavor: tildeLiteral}},
		{"L", tildeModifier{flavor: tildeLiteral}},
		{"i", tildeModifier{fold: true}},
		{"l", tildeModifier{left: true}},
		{"r", tildeModifier{right: true}},
		{"p", tildeModifier{flavor: tildeGlob}},
		{"s", tildeModifier{flavor: tildeGlob}},
		{"g", tildeModifier{greedy: true}},
		{"N", tildeModifier{null: true}},
		{"gN", tildeModifier{greedy: true, null: true}},
		// The toggles reach the two new switches as they reach the fold.
		{"-g", tildeModifier{}},
		{"-N", tildeModifier{}},
		{"g-g", tildeModifier{}},
		{"Ei", tildeModifier{flavor: tildeERE, fold: true}},
		{"iE", tildeModifier{flavor: tildeERE, fold: true}},
		{"Elr", tildeModifier{flavor: tildeERE, left: true, right: true}},
		{"+i", tildeModifier{fold: true}},
		{"-i", tildeModifier{}},
		{"i-i", tildeModifier{}},
		{"-i+i", tildeModifier{fold: true}},
		// The flavor is the last one written rather than the first, which
		// costs nothing to say and is what a `-` before one would otherwise
		// be read as changing.
		{"KE", tildeModifier{flavor: tildeERE}},
		{"EK", tildeModifier{flavor: tildeGlob}},
	} {
		got, unhonored := readTildeModifier(c.body)
		if unhonored != 0 {
			t.Errorf("~(%s): refused %q", c.body, unhonored)
			continue
		}
		if got != c.want {
			t.Errorf("~(%s): %#v, want %#v", c.body, got, c.want)
		}
	}
}

// The group's extent, which is what the lexer handed over and what the matcher
// has to take back off the front of the pattern.
func TestSplittingATildeModifier(t *testing.T) {
	for _, c := range []struct {
		src, body, rest string
		ok              bool
	}{
		{"~(E)a.c", "E", "a.c", true},
		{"~()abc", "", "abc", true},
		{"~(Elr)a.c", "Elr", "a.c", true},
		{"abc", "", "", false},
		{"a~(E)b", "", "", false},
		{"~(E", "", "", false},
		{"~abc", "", "", false},
	} {
		body, rest, ok := splitTildeModifier(c.src)
		if ok != c.ok || body != c.body || rest != c.rest {
			t.Errorf("%q: %q, %q, %v; want %q, %q, %v", c.src, body, rest, ok, c.body, c.rest, c.ok)
		}
	}
}

// tildeGlobPattern is what makes a field a pattern for pathname expansion,
// and the three things it declines are each measured — see the function's own
// rows. Read here as well as through a shell probe, because two of the three
// are absences and a probe that answers "the word stood as written" cannot
// tell an unread group from a group read and found not to matter.
func TestATildeGroupMakesAFieldAPattern(t *testing.T) {
	for _, c := range []struct {
		field string
		group bool
		want  bool
	}{
		{"~(N)a.txt", true, true},
		{"~(i)a.txt", true, true},
		{"~(E)a.txt", true, true},
		// A letter this shell does not answer still makes it one, so the
		// matcher gets to refuse it by name.
		{"~(G)a.txt", true, true},
		// An empty group does not, which is the reference shell's answer for
		// `~()a.txt` and is the control that says this is about letters.
		{"~()a.txt", true, false},
		// Nor does a remainder holding a separator, which is the limit.
		{"~(N)Sub/C.txt", true, false},
		{"~(i)/tmp/x", true, false},
		// Nor a word carrying no group, nor a group that never closes.
		{"a.txt", true, false},
		{"~(N", true, false},
		{"a~(N)b", true, false},
		// And not at all in a grammar without the construct.
		{"~(N)a.txt", false, false},
	} {
		if _, ok := tildeGlobPattern(c.field, c.group); ok != c.want {
			t.Errorf("tildeGlobPattern(%q, %v) = %v, want %v", c.field, c.group, ok, c.want)
		}
	}
}

// A `~(E)` pattern goes to the same engine the `=~` operator uses and is the
// same kind of expression, so a newline in the subject is ordinary ground
// there too — and this is the site a fix for the operator alone would have
// missed.
//
// The rows answering false are the control, exactly as they are for the
// operator: an anchor is about the ends of the subject, and the other flag
// that could have been chosen would turn both of them true. Measured
// 2026-09-20 with `s=$'a\nb'` — `[[ $s == ~(E)a.b ]]` matches there and
// `[[ $s == ~(E)^b ]]` does not. See regexDotAll.
func TestATildeRegexReadsANewlineAsOrdinaryGround(t *testing.T) {
	for _, c := range []struct {
		letters string
		pattern string
		subject string
		want    bool
	}{
		{"E", "a.b", "a\nb", true},
		{"E", "^a.b$", "a\nb", true},
		{"E", "a[^x]+b", "a\nb", true},
		{"E", "^b", "a\nb", false},
		{"E", "a$", "a\nb", false},
		// The fold composes with the flag rather than replacing it, which is
		// the same pair the operator's own rows pin.
		{"Ei", "A.B", "a\nb", true},
		// And a subject with no newline in it answers as it always did.
		{"E", "a.c", "xabcx", true},
	} {
		m, unhonored := readTildeModifier(c.letters)
		if unhonored != 0 {
			t.Fatalf("~(%s) was refused at %c", c.letters, unhonored)
		}
		re, ok := m.tildeRegex(c.pattern, true)
		if !ok {
			t.Fatalf("~(%s)%s did not compile", c.letters, c.pattern)
		}
		if got := re.MatchString(c.subject); got != c.want {
			t.Errorf("~(%s)%s over %q = %v, want %v", c.letters, c.pattern, c.subject, got, c.want)
		}
	}
}
