// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// Words after a `function` keyword's name that are taken and discarded.
//
// The axis is [Dialect.FunctionKeywordReferenceList], where the panel is.
// They are neither more names for the body — that is
// [Dialect.FunctionMultipleNames], and no dialect has both — nor part of it,
// so the tree keeps the first name and nothing else.

// keywordRefList is the grammar this needs: the keyword, the brace-group body
// the same shell wants, and the list itself.
func keywordRefList() Dialect {
	d := Core()
	d.FunctionKeyword = true
	d.FunctionKeywordBodyMustBeBraceGroup = true
	d.FunctionKeywordReferenceList = true
	return d
}

func TestExtraWordsAfterTheNameAreDiscarded(t *testing.T) {
	d := keywordRefList()
	for _, tc := range []struct{ name, src string }{
		{"one word", "function a b { echo hi; }"},
		{"three words", "function a b c d { echo hi; }"},
		{"a quoted word, whose quotes come off first", `function a "b" { echo hi; }`},
		{"a word spelled the same as the name", "function a a { echo hi; }"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := decl(t, tc.src, d)
			if fn.Name != "a" {
				t.Errorf("name is %q, want `a`", fn.Name)
			}
			if len(fn.AlsoNamed) != 0 {
				t.Errorf("%d extra names kept; the list names nothing", len(fn.AlsoNamed))
			}
			if _, isGroup := fn.Body.(*Group); !isGroup {
				t.Errorf("body is %T, want the brace group", fn.Body)
			}
		})
	}
}

func TestTheListStopsAtTheEndOfTheLine(t *testing.T) {
	d := keywordRefList()
	// The rule seen from outside: the words are eaten to the end of the line
	// and the body is looked for after it, so a body that is not a brace
	// group is blamed where it stands rather than where the list began.
	fn := decl(t, "function a echo\n{ echo hi; }", d)
	if fn.Name != "a" {
		t.Errorf("name is %q, want `a`", fn.Name)
	}
	if _, isGroup := fn.Body.(*Group); !isGroup {
		t.Errorf("body is %T, want the brace group", fn.Body)
	}
	// And with nothing but a simple command after the line, there is no body
	// this dialect will take.
	refuses(t, d, "function a echo B\na")
}

func TestOnlyANameMayStandInTheList(t *testing.T) {
	d := keywordRefList()
	// One refusal between them, and it is the list's own rather than the
	// grammar's complaint about the token: a word that is not an identifier,
	// an assignment, and a word that is not text until the shell runs.
	for _, src := range []string{
		"function a 1b { echo hi; }",
		"function a b=c { echo hi; }",
		"function a $foo { echo hi; }",
		"function a 'a b' { echo hi; }",
	} {
		f, err := Parse(src, d)
		_ = f
		if err == nil {
			t.Errorf("%s: parsed, want a refusal", src)
			continue
		}
		if got := err.(*Error).Msg; got != "invalid reference list" {
			t.Errorf("%s: %q, want `invalid reference list`", src, got)
		}
	}
}

func TestAReservedWordEndsTheListRatherThanJoiningIt(t *testing.T) {
	d := keywordRefList()
	// A reserved word is not a name here, and it is refused as the token it
	// is rather than as a bad reference: `function a if { … }` is `` `if'
	// unexpected `` in the shell that has the list. Quoting takes the
	// reservation away, as it does everywhere.
	if _, err := Parse("function a if { echo hi; }", d); err == nil {
		t.Error("`if` was taken as a reference name")
	} else if e := err.(*Error); e.Msg == "invalid reference list" {
		t.Error("`if` was refused as a reference name rather than as a reserved word")
	}
	if fn := decl(t, `function a "if" { echo hi; }`, d); fn.Name != "a" {
		t.Errorf("name is %q, want `a`", fn.Name)
	}
}

func TestWithoutTheListTheSecondWordIsRefused(t *testing.T) {
	d := keywordRefList()
	d.FunctionKeywordReferenceList = false
	// The other five shells' reading, and the one this had: the word after
	// the name is not part of the declaration at all.
	refuses(t, d, "function a b { echo hi; }")
}
