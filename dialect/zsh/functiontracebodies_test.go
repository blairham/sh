// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestATraceMarkFollowsTheBodyItWasWrittenIn is two rows of zsh's own
// E02xtrace (#5156): a nameless function carries the mark of the body it is
// written in, and a function that redefines itself keeps its mark. Every row
// measured 2026-10-01 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`).
func TestATraceMarkFollowsTheBodyItWasWrittenIn(t *testing.T) {
	rows := []struct{ name, src, want string }{
		{
			"nameless bodies trace under -T and a named callee does not",
			"gn() { true }\nfn() { () { gn; () { true } } }\nfunctions -T fn\nfn\n",
			"+fn:0> '(anon)'\n+(anon):0> gn\n+(anon):0> '(anon)'\n+(anon):0> true\n",
		},
		{
			"a nameless body still restores the option",
			"() { set -x }; print after\n",
			"after\n",
		},
		{
			"a function redefining itself keeps -T",
			"f() { f() { echo inner } }\nfunctions -T f; f; which f\n",
			"f () {\n\t# traced\n\techo inner\n}\n",
		},
		{
			"and keeps -t, through two redefinitions",
			"c() { c() { echo C2 }; c() { echo C3 } }; functions -t c; c; which c\n",
			"c () {\n\t# traced\n\techo C3\n}\n",
		},
		{
			"redefined from another function, the mark is gone",
			"f() { g }; g() { f() { echo X } }; functions -T f; f 2>/dev/null; which f\n",
			"f () {\n\techo X\n}\n",
		},
		{
			"and from a nameless body",
			"a() { () { a() { echo A2 } } }; functions -T a; a 2>/dev/null; which a\n",
			"a () {\n\techo A2\n}\n",
		},
		{
			"and from text eval runs",
			"b() { eval 'b() { echo B2 }' }; functions -T b; b 2>/dev/null; which b\n",
			"b () {\n\techo B2\n}\n",
		},
		{
			"another name's mark is still cleared",
			"g() { h() { echo H } }; h() { echo old }; functions -T h; g; which h\n",
			"h () {\n\techo H\n}\n",
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), row.src)
			if out != row.want {
				t.Errorf("got\n%q\nwant\n%q", out, row.want)
			}
		})
	}
}

// TestAFunctionNameTheLocaleCannotPrintIsDollarQuoted is
// Diagnostics.FunctionListingNameDollarQuotes, and the `$'…'` spelling of a
// definition's name it reads back as. Same shell, same day.
func TestAFunctionNameTheLocaleCannotPrintIsDollarQuoted(t *testing.T) {
	rows := []struct{ name, src, want string }{
		{"a written $'' name is decoded", "$'c\\td'() { echo y }; $'c\\td'\n", "y\n"},
		{"and after the keyword", "function $'g\\th' { echo k }; $'g\\th'\n", "k\n"},
		{"a tab", "$'a\\tb'() { : }; functions -- $'a\\tb'\n", "$'a\\tb' () {\n\t:\n}\n"},
		{"a NUL", "$'ba\\0z'() { : }; LC_ALL=C which $'ba\\0z'\n", "$'ba\\C-@z' () {\n\t:\n}\n"},
		{"DEL", "$'a\\x7fb'() { : }; functions -- $'a\\x7fb'\n", "$'a\\C-?b' () {\n\t:\n}\n"},
		{"ESC and return", "$'a\\e\\rb'() { : }; functions -- $'a\\e\\rb'\n", "$'a\\C-[\\C-Mb' () {\n\t:\n}\n"},
		{"a C1 byte", "$'a\\x9fb'() { : }; LC_ALL=C functions -- $'a\\x9fb'\n", "$'a\\M-\\C-_b' () {\n\t:\n}\n"},
		{
			"three UTF-8 bytes under C, two of them C1",
			"$'\\xe3\\x83\\x8c'() { : }; LC_ALL=C functions -- $'\\xe3\\x83\\x8c'\n",
			"$'\xe3\\M-\\C-C\\M-\\C-L' () {\n\t:\n}\n",
		},
		{
			"printable to UTF-8",
			"$'\\xe3\\x83\\x8c'() { : }; LC_ALL=en_US.UTF-8 functions -- $'\\xe3\\x83\\x8c'\n",
			"\xe3\x83\x8c () {\n\t:\n}\n",
		},
		{"a lone 0xa0 is printable to C", "$'a\\xa0b'() { : }; LC_ALL=C functions -- $'a\\xa0b'\n", "a\xa0b () {\n\t:\n}\n"},
		{"and not to UTF-8", "$'a\\xa0b'() { : }; LC_ALL=en_US.UTF-8 functions -- $'a\\xa0b'\n", "$'a\\M- b' () {\n\t:\n}\n"},
		{"U+0085 to UTF-8 is its code point", "$'\\xc2\\x85'() { : }; LC_ALL=en_US.UTF-8 functions -- $'\\xc2\\x85'\n", "$'\\M-\\C-E' () {\n\t:\n}\n"},
		{"a quote and a backslash", "$'a\\\\b\\t'() { : }; functions -- $'a\\\\b\\t'\n", "$'a\\\\b\\t' () {\n\t:\n}\n"},
		{"control: printable names are as before", "'a b'() { : }; functions -- 'a b'\n", "'a b' () {\n\t:\n}\n"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), row.src)
			if out != row.want {
				t.Errorf("got\n%q\nwant\n%q", out, row.want)
			}
		})
	}
}

// TestALoopWritesAPositionalAsItWasGiven: `for 1 in …` writes `$1` with the
// word the loop already expanded, and does not read it a second time. Same
// shell, same day; the suite's loop over a list holding `$'ba\0z'` is where
// it showed.
func TestALoopWritesAPositionalAsItWasGiven(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `for 1 in 'a\\0b' "*" "~" '$x'; do print -r -- "[$1]"; done`+"\n")
	if want := "[a\\\\0b]\n[*]\n[~]\n[$x]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
