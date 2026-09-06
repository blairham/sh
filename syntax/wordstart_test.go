// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// wordStart means *the word being read*, and both halves of that are load
// bearing.
//
// The diagnostic that quotes an unmatched construct back quotes its whole
// word, and the scanner is the only thing that knows where a word began — a
// blank inside quotes or behind a backslash does not end one, so the source
// cannot be searched backwards for it (#1022).
//
// The reason the field is *cleared* is the other half, and it is not
// observable from outside: every failUnmatched today is raised from inside a
// word — asserted by sweeping every prefix of every corpus snippet — so a
// field left behind is never read while it is stale. It would be the moment a
// refusal were raised anywhere else, and the value it handed over would be a
// word that had already been read in full. So the invariant is asserted here,
// where it can be, rather than through a symptom that does not exist yet.
func TestWordStartIsTheWordBeingRead(t *testing.T) {
	l := NewLexer(`echo "a b"$(echo`, Core())
	l.Tokens()
	if l.wordStart.IsValid() {
		t.Errorf("wordStart = %+v after the input was read, want cleared", l.wordStart)
	}
}

// And what a refusal raised outside any word quotes: the construct, because
// there is no word to widen to. The guard cannot be reached through Parse —
// see above — so it is reached directly, which is the only way to say what it
// does.
func TestAnUnmatchedConstructOutsideAWordQuotesItself(t *testing.T) {
	const src = "echo one\n$(echo two"
	l := NewLexer(src, Core())
	l.line = 2
	l.failUnmatched(Pos{Offset: 9, Line: 2, Col: 1}, "$(", ")", "unterminated command substitution")
	se, ok := l.err.(*Error)
	if !ok {
		t.Fatalf("err = %v, want an *Error", l.err)
	}
	if want := "$(echo two"; se.LastToken != want {
		t.Errorf("LastToken = %q, want %q", se.LastToken, want)
	}
}

// The word wins where there is one, and it reaches back past everything the
// word contains. Asserted on the state rather than on a sentence, because
// three of the four dialects word this without the text and would agree
// whatever it held.
func TestAnUnmatchedConstructTakesItsWholeWord(t *testing.T) {
	d := Core()
	d.ProcessSubstitution = true
	for _, c := range []struct{ src, near string }{
		{`v=$(echo hi`, `v=$(echo hi`},
		{`echo a$(echo hi`, `a$(echo hi`},
		{`echo one two $(echo hi`, `$(echo hi`},
		{`echo "a b"$(echo hi`, `"a b"$(echo hi`},
		{`echo a\ b$(echo hi`, `a\ b$(echo hi`},
		{`echo 'a b'$(echo hi`, `'a b'$(echo hi`},
		{`echo x >f$(echo hi`, `f$(echo hi`},
		{`echo one; v=x$(echo hi`, `v=x$(echo hi`},
		{`echo $(echo $(echo hi`, `$(echo $(echo hi`},
		{"echo pad\nv=$(echo hi", `v=$(echo hi`},
	} {
		_, err := Parse(c.src, d)
		se, ok := err.(*Error)
		if !ok {
			t.Errorf("%q: got %v, want an *Error", c.src, err)
			continue
		}
		if se.LastToken != c.near {
			t.Errorf("%q:\n near %q\n want %q", c.src, se.LastToken, c.near)
		}
	}
}
