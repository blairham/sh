// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The POSIX `name()` form takes any word as a name (#1743).
//
// This shell defines such a function, calls it, lists it and removes it. Of
// the other five, four read the definition and refuse the *name* where it
// runs — two carrying on, one fatal, one stopping the script — and dash alone
// refuses to parse it. Every row is a measurement on zsh 5.9.2 taken
// 2026-09-10 from a script file under `env -i`.
//
// The rows separate the rule from a wider set of name *characters*: no such
// set could hold a semicolon or a pipe, which end a word when they are bare.
// The `'f'` row is what says so from the other side — the name is one nobody
// could object to and the quotes are all that is unusual.
func TestAPosixFunctionNameIsAnyWord(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a name holding a space", `'a b'() { printf b; }; 'a b'`, "b"},
		{"the same name written with a backslash", `a\ b() { printf b; }; 'a b'`, "b"},
		{"and written in double quotes", `"a b"() { printf b; }; 'a b'`, "b"},
		{"the empty name", `''() { printf b; }; ''`, "b"},
		{"either spelling of it", `""() { printf b; }; ''`, "b"},
		{"a name holding a semicolon", `'a;b'() { printf b; }; 'a;b'`, "b"},
		{"a name holding a pipe", `'a|b'() { printf b; }; 'a|b'`, "b"},
		{"a name holding a dollar", `w=zz; 'a$b'() { printf b; }; 'a$b'; printf "n=%d" ${#functions}`, "bn=1"},
		{"a quoted pattern is ordinary text", `'a*b'() { printf b; }; 'a*b'`, "b"},
		{"a name holding a tab", "a\\\t" + `b() { printf b; }; $'a\tb'`, "b"},
		{"an ordinary name in quotes", `'f'() { printf b; }; f`, "b"},
		{"a name holding a quoted equals", `'a=b'() { printf b; }; 'a=b'`, "b"},
		{"and one whose equals is escaped", `a\=b() { printf b; }; 'a=b'`, "b"},

		// The listing writes each name back the way the shell does, which is
		// what makes it read back as the definition it describes. Bare, the
		// empty one would head the body with a space and `a b` would read as
		// two words in a construct this form does not have.
		{"the listing quotes a name holding a space", `'a b'() { :; }; functions`, "'a b' () {\n\t:\n}\n"},
		{"and the empty name", `''() { :; }; functions`, "'' () {\n\t:\n}\n"},
		{"typeset -f says the same", `'a b'() { :; }; typeset -f`, "'a b' () {\n\t:\n}\n"},

		// The tables name it, count it and give it up.
		{"it is one function", `'a b'() { :; }; printf "n=%d" ${#functions}`, "n=1"},
		{"whence names it", `'a b'() { :; }; whence -v 'a b'`, "a b is a shell function from zsh\n"},
		{"unfunction removes it", `'a b'() { :; }; unfunction 'a b'; printf "st=%d n=%d" $? ${#functions}`, "st=0 n=0"},

		// The two spellings are one declaration, which is the disagreement
		// this issue is: the keyword form took the name and this one did not.
		{"the keyword form defines the same name", `function 'a b' { printf k; }; 'a b'() { printf p; }; 'a b'; printf "n=%d" ${#functions}`, "pn=1"},

		// And the wild line itself, built the way the plugin builds it: a
		// widget's name out of `$widgets[…]`, quoted with backslashes and
		// handed to `eval` as a `name()` definition.
		{
			"the eval that finds it in the wild",
			`w='_bound__complete_help\ -C\ .complete-word\ _complete_help'; ` +
				`eval "${w}() { printf b; }"; printf "st=%d " $?; ` +
				`'_bound__complete_help -C .complete-word _complete_help'`,
			"st=0 b",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// An array assignment is not a definition of a function whose name ends in
// `=`, and the parentheses follow a word either way.
//
// Measured: `a=() { :; }` is `parse error near '{'` in that shell — the
// assignment wins and the brace group has nothing to be part of — while
// `'a=b'()` and `a\=b()` are definitions, so it is the bare `=` that decides.
func TestAnArrayAssignmentIsNotAPosixDefinition(t *testing.T) {
	dir := t.TempDir()
	if out, st := runZsh(t, dir, `a=(x y); printf "%d %s" ${#a} ${a[1]}`); out != "2 x" || st != 0 {
		t.Errorf("out %q status %d, want %q at 0", out, st, "2 x")
	}
	if _, err := parseZsh(`a=() { :; }`); err == nil {
		t.Error("`a=() { :; }` parsed, want the parse error that shell gives")
	}
}

// A name whose bare text holds a pattern character is refused rather than
// defined, because that shell matches such a word against the filesystem.
func TestABarePatternIsNotAPosixName(t *testing.T) {
	for _, src := range []string{
		`a*b() { :; }`,
		`a?b() { :; }`,
		`a[b() { :; }`,
	} {
		if _, err := parseZsh(src); err == nil {
			t.Errorf("%s: parsed, want a refusal", src)
		}
	}
}
