// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// big5Alpha is U+03B1 as Big5 writes it: a lead byte and a backslash.
//
// Written out as bytes rather than as a Go escape of the character, because
// the bytes are the subject — this is the encoding the *file* is in, which is
// what a locale names and what no Go string literal of the letter would
// produce.
const big5Alpha = "\xa3\x5c"

// big5 is a reader told where a character ends, for the one pair these tests
// use. A hand-written hook rather than internal/charset's, because what is
// under test here is the lexer's *use* of the answer and not the answer: a
// table would make the test depend on a row and prove the same thing.
func big5(d Dialect) Dialect {
	d.CharacterWidth = func(b, next byte) int {
		if b == 0xA3 && next == 0x5C {
			return 2
		}
		return 1
	}
	return d
}

// A character whose second byte is a backslash is read whole, so the byte is
// not an escape.
//
// Both readings are asserted on every shape, because the one with no hook is
// what this shell did and is a reading a dialect still holds — see
// interp.Semantics.MultibyteCharacterIsReadWhole, where zsh, dash and ash
// answer No. So neither column is the absence of behavior, and the *whole*
// token stream is compared rather than a word of it: the escape does not only
// change a word, it changes how many there are.
func TestAMultibyteCharacterIsReadWhole(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		src  string
		// whole is the token stream where the character is one character, and
		// bytes the stream where it is two bytes. An empty bytes is a refusal:
		// the escaped byte was one the grammar needed.
		whole, bytes string
	}{
		{
			// The word from the issue. With the trail byte read as an escape
			// the newline goes with it, `x=` runs into `echo`, and the script
			// is one line saying something nobody wrote — at status 0, with
			// nothing reported.
			"an unquoted word",
			"x=" + big5Alpha + "\necho hi\n",
			"word(x=" + big5Alpha + ") nl word(echo) word(hi) nl",
			"word(x=\xa3echo) word(hi) nl",
		},
		{
			// Inside double quotes the byte the escape swallows is the closing
			// quote, so the rest of the file is quoted and the input runs out.
			"inside double quotes",
			`x="` + big5Alpha + `"` + "\n",
			`word(x=|"` + big5Alpha + `") nl`,
			"",
		},
		{
			// A `case` pattern, where what the escape swallows is the blank
			// in front of `in` — so the pattern list is never reached and the
			// words run together.
			"a case pattern",
			"case " + big5Alpha + " in " + big5Alpha + ") echo m;; esac\n",
			"word(case) word(" + big5Alpha + ") word(in) word(" + big5Alpha +
				") ) word(echo) word(m) ;; word(esac) nl",
			"word(case) word(\xa3|\\ |in) word(\xa3|\\)) word(echo) word(m) ;; word(esac) nl",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			l := NewLexer(c.src, big5(Core()))
			toks := l.Tokens()
			if l.Err() != nil {
				t.Fatalf("reading the character whole: %v", l.Err())
			}
			if got := render(toks); got != c.whole {
				t.Errorf("got  %s\nwant %s", got, c.whole)
			}
			// And the other reading, which has to differ: an assertion that
			// named only the answer above would pass against a lexer that
			// never asked the hook at all.
			plain := NewLexer(c.src, Core())
			ptoks := plain.Tokens()
			if c.bytes == "" {
				if plain.Err() == nil {
					t.Errorf("reading it as bytes: no refusal, got %s", render(ptoks))
				}
				return
			}
			if plain.Err() != nil {
				t.Fatalf("reading it as bytes: %v", plain.Err())
			}
			if got := render(ptoks); got != c.bytes {
				t.Errorf("as bytes got  %s\nwant          %s", got, c.bytes)
			}
		})
	}
}

// An unquoted here-document's body is shell text and is read the same way.
//
// The assertion is that the `$v` stays an *expansion*, which is the silent half
// of this defect: with the trail byte read as an escape the `$` is protected,
// the body carries the two characters `$v` as text, and nothing anywhere
// reports it.
func TestAHeredocBodyReadsTheCharacterWhole(t *testing.T) {
	t.Parallel()
	const body = big5Alpha + "$v\n"
	for _, c := range []struct {
		name  string
		d     Dialect
		spans int
		first string
	}{
		{"read whole", big5(Core()), 3, big5Alpha},
		{"read as bytes", Core(), 1, "\xa3$v\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			spans, err := HeredocSpans(body, c.d)
			if err != nil {
				t.Fatalf("%v", err)
			}
			if len(spans) != c.spans {
				t.Fatalf("body has %d spans, want %d: %#v", len(spans), c.spans, spans)
			}
			if spans[0].Value != c.first {
				t.Errorf("first span is %q, want %q", spans[0].Value, c.first)
			}
			if c.spans == 1 {
				// One span is the whole body as text, which is the wrong
				// answer this exists to name: nothing in it expands.
				return
			}
			if spans[1].Kind != ParamExp {
				t.Errorf("second span is %v, want the expansion the `$v` is", spans[1].Kind)
			}
		})
	}
}

// A lead byte the input does not finish is a byte, and taking it for half a
// character would consume what is not there.
//
// The two shapes that reach the guard: the end of the input, and a newline
// where a trail byte would go. Both matter — the newline is the one a script
// meets, since a file whose last line ends in a character is every file — and
// a reader that consumed the byte after the lead without asking would eat the
// line ending and join two lines.
func TestALeadByteNothingFinishesIsAByte(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		src    string
		tokens int
		first  string
	}{
		{"at the end of the input", "x=\xa3", 2, "x=\xa3"},
		{"before a newline", "x=\xa3\necho hi\n", 6, "x=\xa3"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			l := NewLexer(c.src, big5(Core()))
			toks := l.Tokens()
			if l.Err() != nil {
				t.Fatalf("%v", l.Err())
			}
			if len(toks) != c.tokens {
				t.Errorf("got %d tokens, want %d: %s", len(toks), c.tokens, render(toks))
			}
			if got := toks[0].Literal(); got != c.first {
				t.Errorf("first word is %q, want %q", got, c.first)
			}
		})
	}
}
