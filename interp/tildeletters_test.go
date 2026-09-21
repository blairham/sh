// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
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
		x, ok := m.tildeRegex(c.pattern, true)
		if !ok {
			t.Fatalf("~(%s)%s did not compile", c.letters, c.pattern)
		}
		if got := x.match(c.subject); got != c.want {
			t.Errorf("~(%s)%s over %q = %v, want %v", c.letters, c.pattern, c.subject, got, c.want)
		}
	}
}

// The basic flavor's operators, which are the mirror of the extended one's:
// a backslashed `(`, `{`, `|`, `+` and `?` is the operator and a bare one is
// the character. Read through the translation rather than through a shell
// probe, because the shell's own quote removal reaches a written pattern and
// keeps a different set of backslashes per flavor — so a probe written into a
// condition measures the word as much as the expression, which is the trap
// tildeKeepsBackslash is about.
//
// Every row is measured on ksh93u+ 2012-08-01, 2026-09-20, with the pattern
// supplied through a variable so that nothing reaches it first. See breToRE2
// for the rows in the reference shell's own spelling.
func TestABasicRegularExpressionTranslates(t *testing.T) {
	for _, c := range []struct {
		pattern string
		subject string
		want    bool
	}{
		// The five operators the backslash turns on.
		{`a\(b\)c`, "abc", true},
		{`a(b)c`, "a(b)c", true},
		{`a\{3\}`, "aaa", true},
		{`a\{3\}`, "aa", false},
		{`a{3}`, "a{3}", true},
		{`a\|b`, "ab", true},
		{`a\|b`, "b", true},
		{`a|b`, "a|b", true},
		{`a\+b`, "aab", true},
		{`a+b`, "a+b", true},
		{`ax\?b`, "ab", true},
		{`a?b`, "a?b", true},
		{`\(a\)\{3\}`, "aaa", true},
		{`a\{2,\}`, "aaa", true},
		// What reads as it does anywhere.
		{`a.c`, "abc", true},
		{`a.c`, "xabcx", true},
		{`a\.b`, "a.b", true},
		{`a\.b`, "aXb", false},
		{`[[:alpha:]]*`, "abc", true},
		{`a[]]b`, "a]b", true},
		{`a[a-]b`, "a-b", true},
		{`a[^.]b`, "axb", true},
		{`a\*b`, "a*b", true},
		{`aaa`, "aa", false},
		// The anchors, which are positional: live at the ends and just past
		// a `\(`, and the character anywhere else. The row that pins the
		// last one is `^a\|^b` over `b`, which does **not** match there —
		// so a caret past a `\|` is the character.
		{`^abc`, "xabc", false},
		{`^abc`, "abc", true},
		{`x^y`, "x^y", true},
		{`abc$`, "abcx", false},
		{`abc$`, "abc", true},
		{`x$y`, "x$y", true},
		{`\(^a\)b`, "ab", true},
		{`\(^a\)b`, "xab", false},
		{`a\(b$\)`, "ab", true},
		{`a\(b$\)`, "abx", false},
		{`^a\|^b`, "ab", true},
		{`^a\|^b`, "b", false},
	} {
		for _, letters := range []string{"G", "V"} {
			m, unhonored := readTildeModifier(letters)
			if unhonored != 0 {
				t.Fatalf("~(%s) was refused at %c", letters, unhonored)
			}
			if bad := m.unsupportedTilde(c.pattern); bad != "" {
				t.Fatalf("~(%s)%s: refused for the %s", letters, c.pattern, bad)
			}
			x, ok := m.tildeRegex(c.pattern, true)
			if !ok {
				t.Fatalf("~(%s)%s did not compile", letters, c.pattern)
			}
			if got := x.match(c.subject); got != c.want {
				t.Errorf("~(%s)%s over %q = %v, want %v", letters, c.pattern, c.subject, got, c.want)
			}
		}
	}
}

// The conjunction, whose operands describe the **same span** rather than the
// same subject — which is the reading a plain `a && b` over the whole string
// would get wrong, and the rows answering false are what separate them.
//
// Measured 2026-09-20 on ksh93u+: `[[ abc == ~(X)a&c ]]` does not match even
// though `a` and `c` are each in the subject, and `[[ abcabc ==
// ~(X)^abc&abc$ ]]` does not either, with `[[ abab == ~(X)^ab&ab ]]` and
// `[[ abab == ~(X)ab&ab$ ]]` as the controls that say each anchor alone is
// satisfiable.
func TestAConjunctionTakesOneSpan(t *testing.T) {
	for _, c := range []struct {
		pattern string
		subject string
		want    bool
	}{
		{`a.c&abc`, "abc", true},
		{`a.c&axc`, "abc", false},
		{`(a.c)&(abc)`, "abc", true},
		{`a.&.b`, "ab", true},
		{`b&b`, "ab", true},
		{`ab&b`, "ab", false},
		{`a&c`, "abc", false},
		{`^a&c$`, "abc", false},
		{`^abc&abc$`, "abcabc", false},
		{`^ab&ab`, "abab", true},
		{`ab&ab$`, "abab", true},
		// `&` binds tighter than `|`, which is the one thing here a reader
		// is likely to have the other way round.
		{`a&b|c`, "c", true},
		{`a&(b|c)`, "c", false},
		// An operand with nothing in it describes only the empty span, and
		// no other operand of the same alternative can.
		{`abc&`, "abc", false},
		{`&abc`, "abc", false},
		// A `&` that is not the operator: inside a group, inside a bracket
		// expression, and written `\&`.
		{`(a&b)`, "abc", false},
		{`[&]b`, "a&b", true},
		{`a\&b`, "a&b", true},
		{`a\&b`, "ab", false},
	} {
		m, unhonored := readTildeModifier("X")
		if unhonored != 0 {
			t.Fatalf("~(X) was refused at %c", unhonored)
		}
		x, ok := m.tildeRegex(c.pattern, true)
		if !ok {
			t.Fatalf("~(X)%s did not compile", c.pattern)
		}
		if got := x.match(c.subject); got != c.want {
			t.Errorf("~(X)%s over %q = %v, want %v", c.pattern, c.subject, got, c.want)
		}
		// The same characters under `E`, where `&` is ordinary text. The
		// control that says the operator belongs to the letter.
		if !strings.ContainsAny(c.pattern, "()[\\|") {
			e, _ := readTildeModifier("E")
			if x, ok := e.tildeRegex(c.pattern, true); ok && len(x.alts) != 0 {
				t.Errorf("~(E)%s was read as a conjunction", c.pattern)
			}
		}
	}
}

// What each flavor refuses by name, and — the half that is easy to lose — what
// it does not. A scan that refused everything would pass a test listing only
// refusals, so every row carries the pattern that is the same shape and is
// taken.
func TestAFlavorRefusesTheConstructsItsEngineLacks(t *testing.T) {
	for _, c := range []struct {
		letters string
		pattern string
		want    string
	}{
		{"E", `(ab)\1`, `\1 backreference`},
		{"X", `(ab)\1`, `\1 backreference`},
		{"P", `(ab)\1`, `\1 backreference`},
		{"G", `\(ab\)\1`, `\1 backreference`},
		{"V", `\(ab\)\1`, `\1 backreference`},
		{"E", `a(?=b)bc`, `(?= lookaround`},
		{"X", `a(?=b)bc`, `(?= lookaround`},
		{"P", `(?<=a)bc`, `(?<= lookaround`},
		// The word edges are the basic flavors' own, since RE2 has `\b` and
		// no way to take one side of it. `\<` is measured live there:
		// `[[ 'ab cd' == ~(G)\<cd ]]` matches and `[[ abcd == … ]]` does not.
		{"G", `\<cd`, `\< word edge`},
		{"V", `cd\>`, `\> word edge`},
		// And what is taken. A bare `(?=` is not a lookaround in a basic
		// expression — every character of it is ordinary there — and a
		// `\1` inside a bracket expression is not a backreference anywhere.
		{"G", `a(?=b)c`, ""},
		{"V", `a(?=b)c`, ""},
		{"E", `a[(?=]b`, ""},
		{"X", `a.c&abc`, ""},
		{"P", `a\d\w\s`, ""},
		{"G", `a\(b\)c`, ""},
		{"V", `a\{3\}`, ""},
		{"K", `a\1`, ""},
		{"F", `a\1`, ""},
	} {
		m, unhonored := readTildeModifier(c.letters)
		if unhonored != 0 {
			t.Fatalf("~(%s) was refused at %c", c.letters, unhonored)
		}
		if got := m.unsupportedTilde(c.pattern); got != c.want {
			t.Errorf("~(%s)%s: refused %q, want %q", c.letters, c.pattern, got, c.want)
		}
	}
}

// Which backslashes are the expression's rather than the shell's, per flavor.
//
// The set differs by flavor because the reference shell's does — see
// tildeKeepsBackslash for the measured rows. The rows answering false are
// what make this a reading rather than a list: a span that is not a
// backslash-quoted single character is never one of these, whatever the
// flavor, and a glob has no engine for a backslash to reach.
func TestWhichBackslashesBelongToTheExpression(t *testing.T) {
	backslashed := func(v string) syntax.Span {
		return syntax.Span{Kind: syntax.Literal, Quoting: syntax.BackslashQuoted, Value: v}
	}
	for _, c := range []struct {
		flavor tildeFlavor
		span   syntax.Span
		want   bool
	}{
		{tildeERE, backslashed("1"), true},
		{tildeERE, backslashed("9"), true},
		{tildeERE, backslashed("0"), false},
		{tildeERE, backslashed("."), false},
		{tildeERE, backslashed("&"), false},
		{tildeAugERE, backslashed("1"), true},
		// `X`'s one exception, because there the character is an operator.
		{tildeAugERE, backslashed("&"), true},
		{tildePerl, backslashed("1"), true},
		{tildePerl, backslashed("&"), false},
		{tildeBRE, backslashed("1"), true},
		{tildeBRE, backslashed("("), true},
		{tildeBRE, backslashed(")"), true},
		{tildeBRE, backslashed("|"), true},
		{tildeBRE, backslashed("?"), true},
		{tildeBRE, backslashed("<"), true},
		// Measured dropped by that shell, and dropped here for the same
		// reason the others are kept: the reference is what is reproduced.
		{tildeBRE, backslashed("+"), false},
		{tildeBRE, backslashed("{"), false},
		{tildeBRE, backslashed("."), false},
		// No engine, so no backslash of the expression's.
		{tildeGlob, backslashed("1"), false},
		{tildeLiteral, backslashed("1"), false},
		{tildeNever, backslashed("1"), false},
		// And a span that is not one backslash-quoted character.
		{tildeERE, syntax.Span{Kind: syntax.Literal, Value: "1"}, false},
		{tildeERE, syntax.Span{Kind: syntax.Literal, Quoting: syntax.BackslashQuoted, Value: "12"}, false},
		{tildeBRE, syntax.Span{Kind: syntax.ParamExp, Quoting: syntax.BackslashQuoted, Value: "1"}, false},
	} {
		if got := tildeKeepsBackslash(c.flavor, c.span); got != c.want {
			t.Errorf("flavor %d, span %#v = %v, want %v", c.flavor, c.span, got, c.want)
		}
	}
}
