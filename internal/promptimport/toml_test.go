// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package promptimport

import (
	"strings"
	"testing"
)

// The TOML subset, tested on the shapes a starship configuration is made of
// and on the ones that are refused.
//
// Written rather than imported, for the reason toml.go states: this module's
// runtime dependency surface is zero and several decisions rest on its being
// zero, and what a general package would add is the parts nobody here asks
// for.

func parsed(t *testing.T, text string) *tomlDoc {
	t.Helper()
	doc, err := parseTOML(text)
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	return doc
}

func value(t *testing.T, doc *tomlDoc, table, key string) string {
	t.Helper()
	v, ok := doc.get(table, key)
	if !ok {
		t.Fatalf("[%s] %s is not there", table, key)
	}
	return v.Text
}

// A multi-line basic string, which is how a starship `format` is written and
// is the one construct that runs past the line it starts on.
func TestAMultiLineStringRunsPastItsOwnLine(t *testing.T) {
	doc := parsed(t, "format = \"\"\"\n$a$b\n$c\"\"\"\nafter = 1\n")
	if got := value(t, doc, "", "format"); got != "$a$b\n$c" {
		t.Errorf("format = %q", got)
	}
	// And the reader carries on afterwards, which is the half a test of the
	// string alone would not catch: a reader that lost its place would drop
	// everything after the value rather than failing.
	if got := value(t, doc, "", "after"); got != "1" {
		t.Errorf("the line after the multi-line string was lost: %q", got)
	}
}

// A `#` inside a string is not a comment, and one outside is.
func TestAHashInsideAStringIsNotAComment(t *testing.T) {
	doc := parsed(t, `style = "fg:#1e66f5"   # the color`+"\n")
	if got := value(t, doc, "", "style"); got != "fg:#1e66f5" {
		t.Errorf("style = %q — the comment reader ate part of the value", got)
	}
}

// A literal string carries no escapes, which is why a configuration full of
// backslashes reaches for one.
func TestALiteralStringCarriesNoEscapes(t *testing.T) {
	doc := parsed(t, `format = ' [$symbol$version]($style)\n'`+"\n")
	if got := value(t, doc, "", "format"); got != ` [$symbol$version]($style)\n` {
		t.Errorf("format = %q", got)
	}
}

// A basic string decodes the escapes it may carry, and leaves the ones it
// does not know exactly as written.
func TestABasicStringDecodesWhatItKnowsAndKeepsTheRest(t *testing.T) {
	doc := parsed(t, `a = "one\ntwo"`+"\n"+`b = "kept\qhere"`+"\n")
	if got := value(t, doc, "", "a"); got != "one\ntwo" {
		t.Errorf("a = %q", got)
	}
	if got := value(t, doc, "", "b"); got != `kept\qhere` {
		t.Errorf("b = %q — an unknown escape was guessed at", got)
	}
}

// A flat array, with a separator inside a string that is not a separator.
func TestAnArrayReadsItsElementsAndNotItsCommas(t *testing.T) {
	doc := parsed(t, `detect_files = ["k8s.yaml", "a,b.yaml"]`+"\n")
	v, _ := doc.get("", "detect_files")
	if !v.Is || len(v.List) != 2 || v.List[1] != "a,b.yaml" {
		t.Errorf("detect_files = %+v", v)
	}
}

// Tables, and the keys in the order they were written.
//
// The order matters because a report walks it: a converter whose notes came
// out in a different order on every run would be a converter nobody could
// diff against yesterday's.
func TestTablesAndKeysKeepTheOrderTheyWereWrittenIn(t *testing.T) {
	doc := parsed(t, "root = 1\n\n[b]\nsecond = 2\nfirst = 1\n\n[a]\nx = 1\n")
	if got := strings.Join(doc.names(), ","); got != ",b,a" {
		t.Errorf("tables = %q, want the order the file wrote them", got)
	}
	if got := strings.Join(doc.keys("b"), ","); got != "second,first" {
		t.Errorf("keys = %q, want the order the file wrote them", got)
	}
}

// Everything outside the subset is an error rather than a silent skip.
func TestWhatIsOutsideTheSubsetIsAnErrorAndNotASkip(t *testing.T) {
	for _, tc := range []struct{ name, text string }{
		{"an array of tables", "[[x]]\n"},
		{"a table header that does not close", "[x\n"},
		{"a line that is neither", "just some words\n"},
		{"an assignment with no value", "x =\n"},
		{"a multi-line string that never closes", "x = \"\"\"\nabc\n"},
		{"an array that does not close", "x = [1, 2\n"},
		{"a bare value of two words", "x = two words\n"},
	} {
		if _, err := parseTOML(tc.text); err == nil {
			t.Errorf("%s was accepted", tc.name)
		}
	}
}
