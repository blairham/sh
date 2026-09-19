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
