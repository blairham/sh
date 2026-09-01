// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

func mustFail(t *testing.T, src string, d Dialect, why string) {
	t.Helper()
	if _, err := Parse(src, d); err == nil {
		t.Errorf("%s: %q parsed, want a syntax error", why, src)
	}
}

func mustParse(t *testing.T, src string, d Dialect, why string) {
	t.Helper()
	if _, err := Parse(src, d); err != nil {
		t.Errorf("%s: %q: %v", why, src, err)
	}
}

// TestParenAfterAWordIsASyntaxError covers a rule the whole panel agrees on,
// so it is core rather than a dialect flag. Without it `[[ ( -n x ) ]]` in a
// dialect without `[[` ran as an ordinary command with surprising arguments
// instead of failing.
func TestParenAfterAWordIsASyntaxError(t *testing.T) {
	for _, src := range []string{`echo (`, `echo a (b)`, `function f() { echo x; }`} {
		mustFail(t, src, POSIX(), "paren after a word")
	}
	// In command position it still opens a subshell, and a pipeline or a
	// list may still be followed by one.
	for _, src := range []string{`(echo x)`, `echo x | (cat)`, `echo a; (echo b)`, `{ echo x; }`} {
		mustParse(t, src, Core(), "paren in command position")
	}
}

// TestStrayStopWordIsASyntaxError is why `function f { ...; }` fails in a
// dialect without the keyword: the `}` has nothing open, and parsing used to
// stop there quietly, silently discarding the rest of the script.
func TestStrayStopWordIsASyntaxError(t *testing.T) {
	for _, src := range []string{`}`, `echo hi; }`, `done`, `then`, `function f { echo kw; }`} {
		mustFail(t, src, POSIX(), "stray stop word")
	}
	// The same words still terminate the constructs that open them, and are
	// ordinary arguments where no command may begin.
	for _, src := range []string{
		`case a in a) echo x;; esac`,
		`if true; then echo x; fi`,
		`while false; do echo x; done`,
		`{ echo x; }`,
		`echo then done }`,
	} {
		mustParse(t, src, Core(), "stop word in its place")
	}
}

func TestArrayLiteralIsADialectQuestion(t *testing.T) {
	mustFail(t, `a=(x y)`, POSIX(), "dash has no arrays")
	mustParse(t, `a=(x y)`, Core(), "every other panel shell has arrays")
	// Not adjacent, and not a subshell either: dash, bash and zsh all call
	// `a= (echo x)` a syntax error. ksh93 alone accepts it, which is not
	// enough to make it core.
	mustFail(t, `a= (echo x)`, Core(), "a space does not make it a subshell")
}

// TestFunctionFormsAreSeparateFlags is the flag-level half of what the dialect
// packages assert per shell: the keyword and the hybrid are independent, so a
// dialect can have one without the other.
func TestFunctionFormsAreSeparateFlags(t *testing.T) {
	const kw, hybrid = `function f { echo x; }`, `function f() { echo x; }`
	keywordOnly := Core()
	both := Core()
	both.FunctionKeywordParens = true

	mustParse(t, kw, keywordOnly, "keyword alone")
	mustFail(t, hybrid, keywordOnly, "keyword alone")
	mustParse(t, kw, both, "keyword and parens")
	mustParse(t, hybrid, both, "keyword and parens")
	mustFail(t, kw, POSIX(), "neither")
	mustFail(t, hybrid, POSIX(), "neither")
}

// TestIndirectionIsAFlag likewise. Whether `${!x}` then means the name is a
// semantics question and belongs to the interpreter, not here.
func TestIndirectionIsAFlag(t *testing.T) {
	on := Core()
	on.ParamIndirection = true
	mustParse(t, `echo ${!x}`, on, "flag on")
	mustFail(t, `echo ${!x}`, Core(), "flag off")
	mustFail(t, `echo ${!x}`, POSIX(), "flag off")
}

// TestSelectIsADialectConstruct: `select` is a menu loop in three of the four
// shells and an ordinary word in dash, where the `do` that follows it has
// nothing to open. That makes it a grammar flag rather than a semantic one —
// a dialect *adds* it, and nothing about it conflicts.
func TestSelectIsADialectConstruct(t *testing.T) {
	for _, src := range []string{
		`select x in a b; do echo "$x"; done`,
		`select x; do echo "$x"; done`,
		`select x in a; do :; done < f`,
	} {
		mustParse(t, src, Core(), "select in a dialect that has it")
		mustFail(t, src, POSIX(), "select in a dialect that does not")
	}
	// The name is required and has to be a name, the same rule `for` has.
	mustFail(t, `select; do :; done`, Core(), "select with no name")
	mustFail(t, `select 1x in a; do :; done`, Core(), "select with a name that is not one")
}

// TestSelectKeepsTheAbsentListDistinct is the same distinction ForClause
// draws: without `in` the menu is the positional parameters, and with `in` and
// nothing after it there is no menu at all. A nil slice cannot say which.
func TestSelectKeepsTheAbsentListDistinct(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`select x in a b; do :; done`, "select x in(a,b) do[cmd[:]]"},
		{`select x; do :; done`, "select x no-list do[cmd[:]]"},
		{`select x in; do :; done`, "select x in() do[cmd[:]]"},
	} {
		if got := parse(t, tc.src, Core()); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}
