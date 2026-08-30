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

// TestHybridFunctionFormIsItsOwnFlag pins the distinction the comment on
// FunctionKeyword described before anything enforced it, which let the ksh
// dialect run a definition ksh93 calls a syntax error.
func TestHybridFunctionFormIsItsOwnFlag(t *testing.T) {
	const kw, hybrid = `function f { echo x; }`, `function f() { echo x; }`
	for _, tc := range []struct {
		name        string
		d           Dialect
		wantKeyword bool
		wantParens  bool
	}{
		{"core", Core(), true, false},
		{"bash", Bash(), true, true},
		{"zsh", Zsh(), true, true},
		{"ksh", Ksh(), true, false},
		{"posix", POSIX(), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantKeyword {
				mustParse(t, kw, tc.d, "keyword form")
			} else {
				mustFail(t, kw, tc.d, "keyword form")
			}
			if tc.wantParens {
				mustParse(t, hybrid, tc.d, "hybrid form")
			} else {
				mustFail(t, hybrid, tc.d, "hybrid form")
			}
		})
	}
}

// TestIndirectionParsesWhereTheShellsAcceptIt is the grammar half of the
// three-way divergence; the semantics half is asserted in the interp package.
func TestIndirectionParsesWhereTheShellsAcceptIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		d    Dialect
		want bool
	}{
		{"bash", Bash(), true},
		{"ksh", Ksh(), true},
		{"zsh", Zsh(), false},
		{"posix", POSIX(), false},
		{"core", Core(), false},
	} {
		if tc.want {
			mustParse(t, `echo ${!x}`, tc.d, tc.name)
		} else {
			mustFail(t, `echo ${!x}`, tc.d, tc.name)
		}
	}
}
