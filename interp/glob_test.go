// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/blairham/sh/interp"
)

func TestParameterTrimming(t *testing.T) {
	// Doubling the operator selects the longer match; there is no greediness
	// syntax in the pattern, so the search order is the whole implementation.
	tests := []struct{ src, want string }{
		{`p=a.b.c; printf "%s" "${p#*.}"`, "b.c"},
		{`p=a.b.c; printf "%s" "${p##*.}"`, "c"},
		{`p=a.b.c; printf "%s" "${p%.*}"`, "a.b"},
		{`p=a.b.c; printf "%s" "${p%%.*}"`, "a"},
		// A pattern that does not match removes nothing.
		{`p=abc; printf "%s" "${p#x}"`, "abc"},
		// Patterns are globs, not regular expressions.
		{`p=abc; printf "%s" "${p#[ab]}"`, "bc"},
		{`p=abc; printf "%s" "${p#?}"`, "bc"},
	}
	for _, tc := range tests {
		if got, _ := run(t, tc.src, nil); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestParameterReplacementAndSubstring(t *testing.T) {
	tests := []struct{ src, want string }{
		{`x=a-b-c; printf "%s" "${x/-/+}"`, "a+b-c"},
		{`x=a-b-c; printf "%s" "${x//-/+}"`, "a+b+c"},
		{`x=a-b; printf "%s" "${x/#a/X}"`, "X-b"},
		{`x=a-b; printf "%s" "${x/%b/Y}"`, "a-Y"},
		// Omitting the replacement deletes the match.
		{`x=a-b; printf "%s" "${x/-}"`, "ab"},
		{`x=abcdef; printf "%s" "${x:1:3}"`, "bcd"},
		{`x=abcdef; printf "%s" "${x:2}"`, "cdef"},
		{`x=abc; printf "%s" "${#x}"`, "3"},
	}
	for _, tc := range tests {
		if got, _ := run(t, tc.src, nil); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// globDir builds a directory to expand patterns against.
func globDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{".hidden", "vis", "a.b"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "f"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runIn(t *testing.T, dir, src string) string {
	t.Helper()
	got, _ := run(t, src, func(r *Runner) { r.Dir = dir })
	return got
}

func TestPathnameExpansion(t *testing.T) {
	dir := globDir(t)
	tests := []struct{ name, src, want string }{
		// The restrictions live in the caller, not the matcher: a leading
		// period is skipped unless the pattern has one.
		{"skips a leading period", `printf "[%s]" *`, `[a.b][sub][vis]`},
		{"a period elsewhere is ordinary", `printf "[%s]" *.b`, `[a.b]`},
		{"an explicit period matches", `printf "[%s]" .*den`, `[.hidden]`},
		// No metacharacter matches `/`, so a pattern cannot cross a
		// directory boundary.
		{"does not cross a slash", `printf "[%s]" *f`, `[*f]`},
		{"matches within a component", `printf "[%s]" sub/*`, `[sub/f]`},
		// A pattern matching nothing is passed through unchanged.
		{"no match passes through", `printf "[%s]" nomatch*`, `[nomatch*]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := runIn(t, dir, tc.src); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// A word read in a position that does not match is not matched, and the rule
// has to reach the *nested* expansions or it is not a rule.
//
// `p=${u:-*}` assigns one asterisk in every shell in the panel. It assigned
// the directory listing here, because the assignment's value went through the
// entry point that promises no globbing while the `:-` word inside it built
// its fields through the one that does. `x=*` was right all along, which is
// what kept this hidden: only a *substituted* word reached the matcher.
func TestASubstitutedWordInsideAnUnmatchedPositionIsNotMatched(t *testing.T) {
	dir := globDir(t)
	for _, tc := range []struct{ src, want string }{
		{`unset u; p=${u:-*}; printf "[%s]" "$p"`, `[*]`},
		{`unset u; p=${u:-vis}; printf "[%s]" "$p"`, `[vis]`},
		{`x=*; printf "[%s]" "$x"`, `[*]`},
		// And the positions that *do* match still do, which is what says
		// this narrowed nothing it should not have.
		{`unset u; printf "[%s]" ${u:-*}`, `[a.b][sub][vis]`},
		{`printf "[%s]" *`, `[a.b][sub][vis]`},
	} {
		if got := runIn(t, dir, tc.src); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.src, got, tc.want)
		}
	}
}

func TestQuotingDecidesWhetherAFieldIsAPattern(t *testing.T) {
	// The per-span quoting from tokenization reaching the last stage of the
	// pipeline. A field has to remember which metacharacters were quoted.
	dir := globDir(t)
	if got := runIn(t, dir, `printf "[%s]" "*"`); got != `[*]` {
		t.Errorf(`a quoted star globbed: %s`, got)
	}
	if got := runIn(t, dir, `p="*"; printf "[%s]" "$p"`); got != `[*]` {
		t.Errorf(`a quoted expansion globbed: %s`, got)
	}
	if got := runIn(t, dir, `printf "[%s]" \*`); got != `[*]` {
		t.Errorf(`an escaped star globbed: %s`, got)
	}
	// Unquoted, the result of an expansion *is* globbed — bash and ksh93 do
	// this where zsh does not, which semantics.md records as an axis.
	if got := runIn(t, dir, `p="*.b"; printf "[%s]" $p`); got != `[a.b]` {
		t.Errorf(`an unquoted expansion did not glob: %s`, got)
	}
}
