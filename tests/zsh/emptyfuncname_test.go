// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// After the `function` keyword the word is the name, whatever is in it
// (#1548, #1560).
//
// This shell defines such a function, calls it, lists it and removes it; the
// other four with the keyword parse the line and refuse the *name* where it
// runs, and dash has no keyword. Every row is a measurement on zsh 5.9.2
// taken 2026-09-08.
//
// The rows separate the rule from two things it is not: a wider set of name
// *characters* — no such set could hold a semicolon or a pipe, which end a
// word when they are bare — and the keyword with no name word at all, which
// is the anonymous form and runs its body.
func TestAKeywordFunctionNameIsAnyWord(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"the empty name", `function '' { printf b; }; ''`, "b"},
		{"either spelling of it", `function "" { printf b; }; ''`, "b"},
		{"the hybrid form takes it too", `function '' () { printf b; }; ''`, "b"},
		{"a name holding a space", `function 'a b' { printf b; }; 'a b'`, "b"},
		{"the same name written with a backslash", `function a\ b { printf b; }; 'a b'`, "b"},
		{"a name holding a semicolon", `function 'a;b' { printf b; }; 'a;b'`, "b"},
		{"a name holding a pipe", `function 'a|b' { printf b; }; 'a|b'`, "b"},
		{"a name holding a dollar", `w=zz; function 'a$b' { printf b; }; 'a$b'; printf "n=%d" ${#functions}`, "bn=1"},
		{"a name holding a quote", `function "a'b" { printf b; }; "a'b"`, "b"},
		{"a name of punctuation", `function '@#%' { printf b; }; '@#%'`, "b"},
		{"a quoted pattern is ordinary text", `function 'a*b' { printf b; }; 'a*b'`, "b"},
		{"and the eval that finds it in the wild", `f='a b'; eval "function ${(q)f} { printf b; }"; printf "st=%d" $?; 'a b'`, "st=0b"},

		// The listing writes each name back the way the shell does, which is
		// what makes it read back as the definition it describes. Bare, the
		// empty one would head the body with a space and read as an
		// anonymous function, and `a b` would read as two names.
		{"the listing quotes the empty name", `function '' { :; }; functions`, "'' () {\n\t:\n}\n"},
		{"and a name holding a space", `function 'a b' { :; }; functions`, "'a b' () {\n\t:\n}\n"},
		{"and a name holding a quote", `function "a'b" { :; }; functions`, "'a'\\''b' () {\n\t:\n}\n"},
		{"and a name of punctuation", `function '@#%' { :; }; functions`, "'@#%' () {\n\t:\n}\n"},
		{"but not a punctuated name that reads back bare", `function a-b { :; }; functions`, "a-b () {\n\t:\n}\n"},
		{"nor one holding a bang or a percent", `function 'a!b' { :; }; functions`, "a!b () {\n\t:\n}\n"},
		{"typeset -f says the same", `function 'a b' { :; }; typeset -f`, "'a b' () {\n\t:\n}\n"},
		{"and a name asked for by hand", `function '' { :; }; functions ''`, "'' () {\n\t:\n}\n"},

		// The tables name it, count it and give it up.
		{"it is one function", `function '' { :; }; printf "n=%d" ${#functions}`, "n=1"},
		{"whence names it", `function '' { :; }; whence -v ''`, " is a shell function from zsh\n"},
		{"unfunction removes it", `function 'a b' { :; }; unfunction 'a b'; printf "st=%d n=%d" $? ${#functions}`, "st=0 n=0"},

		// An empty name arriving through an expansion. Quoted it is a word
		// and defines the function; unquoted it is no word at all, and the
		// keyword is left with an empty name *list*, which defines nothing
		// and is silent at 0. These two are the pair that says the name and
		// the word list are different questions.
		{"a quoted empty expansion names it", `n=""; function "$n" { printf b; }; ''`, "b"},
		{"an unquoted one defines nothing", `n=""; function $n { printf x; }; printf "after n=%d" ${#functions}`, "after n=0"},
		// And the keyword with no name word written at all is the anonymous
		// function, which runs immediately and defines nothing.
		{"no name word at all is the anonymous form", `function { printf a; }; printf "n=%d" ${#functions}`, "an=0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
