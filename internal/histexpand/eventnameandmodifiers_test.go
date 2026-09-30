// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package histexpand

import (
	"errors"
	"testing"
)

// seeded is the one-line list every row below runs against, which is what a
// script holding
//
//	set -o history
//	set -H
//	echo one two
//	<the row>
//
// has when it reaches the row.
func seeded(lines ...string) List { return List{Lines: lines, First: 1} }

// expanded runs one line and reports what it became, failing the test on an
// error.
func expanded(t *testing.T, in string, h List, c Chars) string {
	t.Helper()
	got, err := Expand(in, h, c)
	if err != nil {
		t.Fatalf("expand %q: %v", in, err)
	}
	return got.Line
}

// refused runs one line and reports the error it raised, failing the test when
// there is none.
func refused(t *testing.T, in string, h List, c Chars) error {
	t.Helper()
	got, err := Expand(in, h, c)
	if err == nil {
		t.Fatalf("expand %q: no error, became %q", in, got.Line)
	}
	return err
}

// A backslash after the event character is part of the event's **name**, in
// every column that has an expander.
//
// Measured 2026-09-18 against a one-line list, in a script on bash 5.3.20 and
// at a prompt on zsh 5.9.2 and ksh93u+: `echo T!\xE` is `!\xE: event not
// found` in all three, and inside double quotes `echo "T!\x E"` names `!\x` in
// all three. It was punctuation here, which made the name empty and left the
// `!` as text — so a line nobody could have meant literally ran anyway (#3421).
//
// Unanimous, so it is the engine's answer and not an axis. The backslash in
// front of a `!` is the separate rule the scanner still holds: `echo a\!b` is
// left alone, and the row below keeps the two apart.
func TestABackslashAfterTheEventCharacterIsPartOfTheName(t *testing.T) {
	h := seeded("echo one two")
	for _, c := range []Chars{Default, {Event: '!', Quick: '^', Comment: '#', Words: WordsShell, QuoteIsText: true}} {
		var notFound *NotFound
		if err := refused(t, `echo T!\xE`, h, c); !errors.As(err, &notFound) || notFound.Ref != `!\xE` {
			t.Errorf(`echo T!\xE: %v, want the reference !\xE not found`, err)
		}
		if err := refused(t, `echo "T!\x E"`, h, c); !errors.As(err, &notFound) || notFound.Ref != `!\x` {
			t.Errorf(`echo "T!\x E": %v, want the reference !\x not found`, err)
		}
		// And a backslash in **front** of the event character still holds
		// it off, which is the rule this one is easy to confuse with.
		if got := expanded(t, `echo a\!b`, h, c); got != `echo a\!b` {
			t.Errorf(`echo a\!b became %q, want it left alone`, got)
		}
	}
}

// A single quote or a backquote against the event character is part of the
// name in two columns and ordinary text in the third. See Chars.QuoteIsText.
func TestAQuoteAfterTheEventCharacter(t *testing.T) {
	h := seeded("echo one two")
	part := Chars{Event: '!', Quick: '^', Comment: '#', Words: WordsShell}
	text := part
	text.QuoteIsText = true

	for _, in := range []string{`echo T!'xE'`, "echo T!`xE`"} {
		var notFound *NotFound
		if err := refused(t, in, h, part); !errors.As(err, &notFound) {
			t.Errorf("%s with the quote taken as a name: %v, want an event not found", in, err)
		}
		if got := expanded(t, in, h, text); got != in {
			t.Errorf("%s with the quote taken as text became %q, want it left alone", in, got)
		}
	}
	// A double quote ends a name in both readings, which is what says this
	// axis is about the other two characters and not about quoting.
	var notFound *NotFound
	for _, c := range []Chars{part, text} {
		if err := refused(t, `echo "!\"`, h, c); !errors.As(err, &notFound) || notFound.Ref != `!\` {
			t.Errorf(`echo "!\": %v, want the reference !\ not found`, err)
		}
	}
}

// A second event character inside an event name is a letter of it in two
// columns and closes the name — itself included — in the third. See
// Chars.EventCharClosesAnEventName.
func TestAnEventCharacterInsideAnEventName(t *testing.T) {
	h := seeded("echo one two")
	letter := Chars{Event: '!', Quick: '^', Comment: '#', Words: WordsShell}
	closes := letter
	closes.EventCharClosesAnEventName = true

	var notFound *NotFound
	if err := refused(t, "echo X!ab!cdY", h, letter); !errors.As(err, &notFound) || notFound.Ref != "!ab!cdY" {
		t.Errorf("a letter of the name: %v, want the reference !ab!cdY not found", err)
	}
	if err := refused(t, "echo X!ab!cdY", h, closes); !errors.As(err, &notFound) || notFound.Ref != "!ab!" {
		t.Errorf("closing the name: %v, want the reference !ab! not found", err)
	}
}

// `!{…}` is a braced event reference in one column and is not punctuation at
// all in the other two. See Chars.BracedEvent.
func TestABracedEventReference(t *testing.T) {
	h := seeded("echo abc")
	plain := Chars{Event: '!', Quick: '^', Comment: '#', Words: WordsShell}
	braced := plain
	braced.BracedEvent = true

	if got := expanded(t, "echo X!{!!}Y", h, braced); got != "echo Xecho abcY" {
		t.Errorf("the braced reading became %q, want the event inside the braces", got)
	}
	var notFound *NotFound
	if err := refused(t, "echo X!{!!}Y", h, plain); !errors.As(err, &notFound) || notFound.Ref != "!{!!}Y" {
		t.Errorf("the plain reading: %v, want the whole run taken as a name", err)
	}
	// The empty braces are the same row from the other side: an event named
	// `{}Y` rather than a reference holding nothing.
	if err := refused(t, "echo X!{}Y", h, plain); !errors.As(err, &notFound) || notFound.Ref != "!{}Y" {
		t.Errorf("the empty braces: %v, want the whole run taken as a name", err)
	}
}

// A `$` word designator ends the designator in two columns, so that what
// follows it is text rather than a range. See Chars.LastWordEndsTheDesignator.
func TestTheLastWordEndsTheDesignator(t *testing.T) {
	h := seeded("echo a b c d e")
	ranged := Chars{Event: '!', Quick: '^', Comment: '#', Words: WordsShell}
	ends := ranged
	ends.LastWordEndsTheDesignator = true

	for _, row := range []struct{ in, want string }{
		{"echo !!:$-3", "echo e-3"},
		{"echo !!:$-", "echo e-"},
		{"echo !!:$*", "echo e*"},
	} {
		if got := expanded(t, row.in, h, ends); got != row.want {
			t.Errorf("%s became %q, want %q", row.in, got, row.want)
		}
	}
	// With the other reading the `-` opens a range that runs backwards, and
	// the `*` takes the rest of the words there are.
	var bad *BadWordSpecifier
	if err := refused(t, "echo !!:$-3", h, ranged); !errors.As(err, &bad) {
		t.Errorf("the ranged reading: %v, want a bad word specifier", err)
	}
	if got := expanded(t, "echo !!:$*", h, ranged); got != "echo e" {
		t.Errorf("the ranged reading of `$*` became %q, want the `*` read as part of it", got)
	}
}

// The quick-substitution character names word one where a range's **end** is
// written, in two columns. See Chars.FirstWordEndsARange.
func TestTheFirstWordWhereARangeEnds(t *testing.T) {
	h := seeded("echo a b c d e")
	text := Chars{Event: '!', Quick: '^', Comment: '#', Words: WordsShell}
	word := text
	word.FirstWordEndsARange = true

	if got := expanded(t, "echo !!:1-^", h, word); got != "echo a" {
		t.Errorf("the word reading became %q, want word one at both ends", got)
	}
	var bad *BadWordSpecifier
	if err := refused(t, "echo !!:2-^", h, word); !errors.As(err, &bad) || bad.Ref != ":2-^" {
		t.Errorf("a backwards range: %v, want :2-^ refused as a bad word specifier", err)
	}
	if got := expanded(t, "echo !!:1-^", h, text); got != "echo a b c d^" {
		t.Errorf("the text reading became %q, want `1-` and then the character left alone", got)
	}
}

// A `G` in front of a substitution substitutes once in each word, in one
// column. See Chars.WordwiseSubstitution.
func TestTheWordwiseSubstitutionModifier(t *testing.T) {
	h := seeded("echo foo boo")
	plain := Chars{Event: '!', Quick: '^', Comment: '#', Words: WordsShell}
	wordwise := plain
	wordwise.WordwiseSubstitution = true

	// Once in each word, which `foo` is the discriminator for: a plain `g`
	// would take its second `o` too.
	if got := expanded(t, "echo !!:Gs/o/0/", h, wordwise); got != "echo ech0 f0o b0o" {
		t.Errorf("the wordwise reading became %q, want one substitution in each word", got)
	}
	if got := expanded(t, "echo !!:gs/o/0/", h, wordwise); got != "echo ech0 f00 b00" {
		t.Errorf("`g` became %q, want every substitution", got)
	}
	// And the letter is refused where the axis is off, named as it is
	// written.
	var bad *BadModifier
	if err := refused(t, "echo !!:Gs/o/0/", h, plain); !errors.As(err, &bad) || bad.Mod != "G" {
		t.Errorf("the plain reading: %v, want G refused by name", err)
	}
	// It stands alone: the letter that arrives second is the one named.
	if err := refused(t, "echo !!:gGs/o/0/", h, wordwise); !errors.As(err, &bad) || bad.Mod != "G" {
		t.Errorf("`gG`: %v, want G named", err)
	}
	if err := refused(t, "echo !!:Ggs/o/0/", h, wordwise); !errors.As(err, &bad) || bad.Mod != "g" {
		t.Errorf("`Gg`: %v, want g named", err)
	}
	// A `G` with nothing after it leaves the chain empty, which is the
	// refusal that names nothing rather than one that names the letter.
	if err := refused(t, "echo !!:G", h, wordwise); !errors.As(err, &bad) || bad.Mod != "" {
		t.Errorf("`:G` alone: %v, want a refusal naming nothing", err)
	}
}

// A failed modifier is named after the **whole chain** it stands in, not after
// itself.
//
// Measured 2026-09-18 after `echo one/two.one`, in a script on bash 5.3.20 and
// at a prompt on ksh93u+: `!!:t:gs/x/y/` is `:t:gs/x/y/: substitution failed`
// and `!!:q:&` is `:q:&` in both. Naming the failing modifier alone was this
// engine's answer and is nobody's, so it is the engine's fix rather than an
// axis — the third column names nothing and reads neither.
func TestAFailedModifierNamesTheWholeChain(t *testing.T) {
	c := Chars{Event: '!', Quick: '^', Comment: '#', Words: WordsShell}
	var failed *SubstFailed
	if err := refused(t, "echo !!:t:gs/x/y/", seeded("echo one/two.one"), c); !errors.As(err, &failed) || failed.Ref != ":t:gs/x/y/" {
		t.Errorf("a failed substitution: %v, want the chain :t:gs/x/y/", err)
	}
	// Bare is the spelling one column uses, and for a chain it is the chain:
	// only a quick substitution has a second spelling to report.
	if failed != nil && failed.Bare != ":t:gs/x/y/" {
		t.Errorf("the bare spelling is %q, want the chain", failed.Bare)
	}
	var none *NoPreviousSubstitution
	if err := refused(t, "echo !!:q:&", seeded("echo one two"), c); !errors.As(err, &none) || none.Ref != ":q:&" {
		t.Errorf("an `&` with nothing to repeat: %v, want the chain :q:&", err)
	}
	// A word designator is not a modifier and stays out of the chain.
	if err := refused(t, "echo !!:1:s/x/y/", seeded("echo one two"), c); !errors.As(err, &failed) || failed.Ref != ":s/x/y/" {
		t.Errorf("a designator in front of the chain: %v, want :s/x/y/", err)
	}
	// A quick substitution keeps both spellings, which is the row that says
	// Bare is about that form rather than about trimming a prefix.
	if err := refused(t, "^nosuch^x^", seeded("echo one two"), c); !errors.As(err, &failed) || failed.Ref != ":s^nosuch^x^" || failed.Bare != "^nosuch^x^" {
		t.Errorf("a quick substitution: %v, want :s^nosuch^x^ and ^nosuch^x^", err)
	}
}

// A backslash in a substitution's replacement escapes what follows it, and it
// happens **twice** — in one column and not the other.
//
// See Chars.SubstitutionUnescapesTheReplacement for the panel. The rows here
// are the ones that pin the rule rather than merely agree with it:
//
//   - **The count is not the rule.** `n/4` and two rounds of `n/2` are the
//     same arithmetic, so no number of counted rows can tell them apart. The
//     mixed row can: two backslashes, `a`, four backslashes, `b` comes back
//     `a\b`, which is the two-round reading applied character by character.
//   - **Where the `&` is resolved.** It is the second round, not before or
//     after: `\\&` is a literal `&` while `\\\\&` is a backslash followed by
//     the *match*. Any other order gets one of those two wrong.
//   - **A lone trailing backslash is dropped**, which is why an odd count
//     answers as the even one below it.
//
// Measured 2026-09-30 at a prompt, zsh 5.9.2 against bash 5.3.20 and 3.2.57.
func TestASubstitutionReplacementUnescapesTwice(t *testing.T) {
	h := seeded("echo one two one")
	keeps := Chars{Event: '!', Quick: '^', Comment: '#', Words: WordsShell}
	eats := keeps
	eats.SubstitutionUnescapesTheReplacement = true

	for _, row := range []struct{ in, eaten, kept string }{
		// The counted rows, which say where the first backslash survives.
		{`echo !!:s/o/\X/`, "echo echX one two one", `echo ech\X one two one`},
		{`echo !!:s/o/\\X/`, "echo echX one two one", `echo ech\\X one two one`},
		{`echo !!:s/o/\\\X/`, "echo echX one two one", `echo ech\\\X one two one`},
		{`echo !!:s/o/\\\\X/`, `echo ech\X one two one`, `echo ech\\\\X one two one`},
		{`echo !!:s/o/\\\\\\\\X/`, `echo ech\\X one two one`, `echo ech\\\\\\\\X one two one`},
		// A replacement whose first round leaves a lone backslash at the
		// end, which is the only way that branch is reached: a shell line
		// cannot deliver a trailing backslash, because the reader takes it
		// as a continuation first.
		{`echo !!:s/o/X\\`, "echo echX one two one", `echo echX\\ one two one`},
		// The row a count cannot express.
		{`echo !!:s/o/\\a\\\\b/`, `echo echa\b one two one`, `echo ech\\a\\\\b one two one`},
		// And the two that fix where the `&` is resolved.
		//
		// The kept column is measured and reads oddly on purpose: bash emits
		// each backslash that is not in front of an `&` on its own, so four
		// backslashes and an `&` come back as **three** and an `&` — the pair
		// it consumes is the last one. Deriving that column instead of
		// measuring it is how this row was first written wrong.
		{`echo !!:s/o/\\&/`, "echo ech& one two one", `echo ech\& one two one`},
		{`echo !!:s/o/\\\\&/`, `echo ech\o one two one`, `echo ech\\\& one two one`},
	} {
		if got := expanded(t, row.in, h, eats); got != row.eaten {
			t.Errorf("unescaping twice, %s became %q, want %q", row.in, got, row.eaten)
		}
		if got := expanded(t, row.in, h, keeps); got != row.kept {
			t.Errorf("keeping them, %s became %q, want %q", row.in, got, row.kept)
		}
	}
}

// And the quick form `^old^new^` reads its replacement by the same rule.
//
// Measured separately rather than assumed: `^o^\\\\X^` comes back with one
// backslash in zsh 5.9.2 and four in bash 5.3.20, the same split as the long
// form. A rule threaded into only one of the two call sites passes every row
// above.
func TestTheQuickSubstitutionUnescapesItsReplacementToo(t *testing.T) {
	h := seeded("echo one two one")
	keeps := Chars{Event: '!', Quick: '^', Comment: '#', Words: WordsShell}
	eats := keeps
	eats.SubstitutionUnescapesTheReplacement = true

	if got := expanded(t, `^o^\\\\X^`, h, eats); got != `ech\X one two one` {
		t.Errorf("unescaping twice, the quick form became %q", got)
	}
	if got := expanded(t, `^o^\\\\X^`, h, keeps); got != `ech\\\\X one two one` {
		t.Errorf("keeping them, the quick form became %q", got)
	}
}

// A `:h` or `:t` takes a count of components to keep, in one column and not
// the other.
//
// See Chars.HeadAndTailTakeACount for the panel. The rows are chosen for what
// they decide rather than for coverage:
//
//   - **Zero is the bare modifier**, not "keep none".
//   - **No sign is read**, so `h-1` is the bare `h` with `-1` left as text —
//     which is also what says the digits are scanned and not parsed.
//   - The two letters **part at the far boundary**: `h` past the component
//     count answers with the whole text and `t` past it refuses, *unless* the
//     text is absolute. Four subjects are here because any one of them agrees
//     with a simpler rule; `a/b/c/d` against `/a/b/c` is the pair that
//     separates "past the count" from "past the slashes".
//   - A **trailing slash** is dropped before counting, and then the two
//     letters disagree once more about what "the whole" is.
func TestCountedHeadAndTail(t *testing.T) {
	plain := Chars{Event: '!', Quick: '^', Comment: '#', Words: WordsShell}
	counted := plain
	counted.HeadAndTailTakeACount = true

	for _, row := range []struct{ subj, mod, want, bare string }{
		{"/my/path/for/testing", "h1", "/", "/my/path/for1"},
		{"/my/path/for/testing", "h2", "/my", "/my/path/for2"},
		{"/my/path/for/testing", "h4", "/my/path/for", "/my/path/for4"},
		{"/my/path/for/testing", "h5", "/my/path/for/testing", "/my/path/for5"},
		{"/my/path/for/testing", "h0", "/my/path/for", "/my/path/for0"},
		{"/my/path/for/testing", "t1", "testing", "testing1"},
		{"/my/path/for/testing", "t2", "for/testing", "testing2"},
		{"/my/path/for/testing", "t4", "my/path/for/testing", "testing4"},
		{"/my/path/for/testing", "t5", "/my/path/for/testing", "testing5"},
		{"/my/path/for/testing", "t0", "testing", "testing0"},
		// Chained, which is what says the count applies to the result of the
		// modifier before it rather than to the original word.
		{"/my/path/for/testing", "t3:h2", "path/for", "testing3:h2"},
		{"/my/path/for/testing", "h2:t1", "my", "/my/path/for2:t1"},
		// No sign, and the run ends at the first non-digit.
		{"/my/path/for/testing", "h-1", "/my/path/for-1", "/my/path/for-1"},
		{"/my/path/for/testing", "h01", "/", "/my/path/for01"},
		{"/my/path/for/testing", "h1x", "/x", "/my/path/for1x"},
		// The boundary, relative against absolute.
		{"a/b/c/d", "t3", "b/c/d", "d3"},
		{"/a/b/c", "t3", "a/b/c", "c3"},
		{"/a/b/c", "t4", "/a/b/c", "c4"},
		// A trailing slash is dropped before counting, and `h`'s whole keeps
		// it where `t`'s does not.
		{"/a/b/", "t1", "b", ""},
		{"/a/b/", "t2", "a/b", ""},
		{"/a/b/", "h3", "/a/b/", ""},
		{"/a/b/", "t3", "/a/b", ""},
	} {
		list := seeded("echo " + row.subj)
		in := "echo !1:1:" + row.mod
		if got := expanded(t, in, list, counted); got != "echo "+row.want {
			t.Errorf("counted, %s over %q became %q, want %q", in, row.subj, got, "echo "+row.want)
		}
		if row.bare == "" {
			continue
		}
		if got := expanded(t, in, list, plain); got != "echo "+row.bare {
			t.Errorf("bare, %s over %q became %q, want %q", in, row.subj, got, "echo "+row.bare)
		}
	}
}

// And a counted `:t` asking for more than the text has **refuses**, where the
// same count on an absolute path does not.
//
// The pair is the point: a row that only asserted the refusal would pass an
// implementation that refused for every over-count, and a row that only
// asserted the absolute case would pass one that never refused.
func TestACountedTailPastTheTextRefuses(t *testing.T) {
	counted := Chars{Event: '!', Quick: '^', Comment: '#', Words: WordsShell}
	counted.HeadAndTailTakeACount = true

	var failed *ModifierFailed
	if err := refused(t, "echo !1:1:t3", seeded("echo a/b/c"), counted); !errors.As(err, &failed) {
		t.Errorf("a relative path over-counted: %v, want a failed modifier", err)
	} else if failed.Mod != "t" {
		t.Errorf("the failure names %q, want t", failed.Mod)
	}
	if got := expanded(t, "echo !1:1:t4", seeded("echo /a/b/c"), counted); got != "echo /a/b/c" {
		t.Errorf("an absolute path over-counted became %q, want the whole text", got)
	}
	// And the bare modifier never refuses, which is what keeps the refusal
	// scoped to the counted form.
	plain := counted
	plain.HeadAndTailTakeACount = false
	if got := expanded(t, "echo !1:1:t3", seeded("echo a/b/c"), plain); got != "echo c3" {
		t.Errorf("the bare reading became %q, want the digits left as text", got)
	}
}
