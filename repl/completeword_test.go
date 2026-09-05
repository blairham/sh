// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// Where a word begins is a question about everything to its left: the space
// in `"a b` is inside a quotation and the one in `a b` is not.
func TestWordSpan(t *testing.T) {
	for _, c := range []struct {
		line      string
		wantStart int
		wantQuote rune
	}{
		{"", 0, 0},
		{"echo abc", 5, 0},
		{"echo  ", 6, 0},
		{`echo a\ b`, 5, 0},
		{"ls /usr/bi", 3, 0},
		{`echo "a b`, 5, '"'},
		{`echo "a b" c`, 11, 0},
		{`echo 'a b`, 5, '\''},
		{`echo 'a b' c`, 11, 0},
		// A quote inside double quotes is an ordinary character, and so is a
		// double quote inside single quotes.
		{`echo "a'b`, 5, '"'},
		{`echo 'a"b`, 5, '\''},
		// The grammar's own word enders, with and without space around them.
		{"ls|gr", 3, 0},
		{"ls;gr", 3, 0},
		{"ls && gr", 6, 0},
		{"cat >out", 5, 0},
		{"(gr", 1, 0},
		// Quoted, they are part of the word rather than the end of it.
		{`echo "a;b`, 5, '"'},
		{`echo a\;b`, 5, 0},
		// A backslash inside double quotes escapes only some characters, but
		// it never ends the word, so the pair is skipped either way.
		{`echo "a\"b`, 5, '"'},
	} {
		start, quote := wordSpan([]rune(c.line), len([]rune(c.line)))
		if start != c.wantStart || quote != c.wantQuote {
			t.Errorf("%q: got (%d, %q), want (%d, %q)",
				c.line, start, quote, c.wantStart, c.wantQuote)
		}
	}
}

// The literal text a typed word names, which is what gets matched against
// what is on the disk.
func TestDequote(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"plain", "plain"},
		{`a\ b`, "a b"},
		{`"a b`, "a b"},
		{`'a b`, "a b"},
		{`"a'b`, "a'b"},
		{`'a"b`, `a"b`},
		{`'a'\''b`, "a'b"},
		{`"a\$b`, "a$b"},
		// Inside double quotes a backslash before an ordinary character is
		// an ordinary backslash, which is why `"a\ b` is not `a b`.
		{`"a\ b`, `a\ b`},
		{`\~lead`, "~lead"},
		{`a\`, "a"},
	} {
		if got := dequote(c.in); got != c.want {
			t.Errorf("dequote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A name has to be written so that reading the line back gives the name
// again, and what that takes depends on the quotation it lands inside.
func TestEscapeName(t *testing.T) {
	for _, c := range []struct {
		name    string
		quote   rune
		atStart bool
		want    string
	}{
		{"plain.txt", 0, true, "plain.txt"},
		{"file one.txt", 0, false, `file\ one.txt`},
		{"file one.txt", '"', false, "file one.txt"},
		{"file one.txt", '\'', false, "file one.txt"},
		{"quo'te.txt", 0, false, `quo\'te.txt`},
		{"quo'te.txt", '"', false, "quo'te.txt"},
		{"quo'te.txt", '\'', false, `quo'\''te.txt`},
		{"a$b.txt", 0, false, `a\$b.txt`},
		{"a$b.txt", '"', false, `a\$b.txt`},
		{"a$b.txt", '\'', false, "a$b.txt"},
		{"back`tick", '"', false, "back\\`tick"},
		{"bracket[1].txt", 0, false, `bracket\[1\].txt`},
		// `~` and `#` mean something only at the start of an unquoted word.
		{"~lead.txt", 0, true, `\~lead.txt`},
		{"~lead.txt", 0, false, "~lead.txt"},
		{"~lead.txt", '"', true, "~lead.txt"},
		{"#lead.txt", 0, true, `\#lead.txt`},
		{"a~b#c", 0, true, "a~b#c"},
		// Neither shell escapes a leading dash, and neither does this.
		{"-lead.txt", 0, true, "-lead.txt"},
	} {
		if got := escapeName(c.name, c.quote, c.atStart); got != c.want {
			t.Errorf("escapeName(%q, %q, %v) = %q, want %q",
				c.name, c.quote, c.atStart, got, c.want)
		}
	}
}

// The part of the word every candidate keeps: the directory typed so far,
// along with an opening quote.
func TestWordPrefix(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"pla", ""},
		{"sub/nes", "sub/"},
		{"a/b/c", "a/b/"},
		{`"sub/nes`, `"sub/`},
		{`"file`, `"`},
		{`'file`, `'`},
		{"~/Deve", "~/"},
		{`a\/b`, ""},
		{`'a\/b`, `'a\/`},
	} {
		if got := wordPrefix(c.in); got != c.want {
			t.Errorf("wordPrefix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The common prefix is taken in escaped units, because a backslash and what
// it escapes are one thing.
func TestCommonPrefixKeepsAnEscapeWhole(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{[]string{`x\$a`, `x\&b`}, "x"},
		{[]string{`file\ one.txt`, `file\ two.txt`}, `file\ `},
		{[]string{`'a'\''x`, `'a'\''y`}, `'a'\''`},
		{[]string{"apple", "apricot"}, "ap"},
		{[]string{"日本語", "日本"}, "日本"},
	} {
		if got := commonPrefix(c.in); got != c.want {
			t.Errorf("commonPrefix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
