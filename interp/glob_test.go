// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
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

// TestADotComponentIsJoinedNotMatched is #1480: a `.` or `..` component made
// the whole pattern match nothing, so `./*` listed no files at all — and in
// the dialect where an unmatched pattern is fatal, ended the script.
//
// Two things have to hold, and the second is the one a partial fix loses. The
// component has to be *joined* rather than matched, because no listing
// reports `.` or `..`; and the match has to come back spelled the way the
// pattern spelled it, because the panel answers `./sub/f` where a cleaning
// join answers `sub/f`. So every row here asserts the whole string rather
// than a count: a count is satisfied by the wrong paths.
func TestADotComponentIsJoinedNotMatched(t *testing.T) {
	dir := globDir(t)
	for _, tc := range []struct{ name, src, want string }{
		// A leading `./`, which is the everyday spelling.
		{"leading dot", `printf "[%s]" ./*`, `[./a.b][./sub][./vis]`},
		{"leading dot, two components", `printf "[%s]" ./sub/*`, `[./sub/f]`},
		// The same component away from the front, and `..` — which is
		// where the spelling has to be carried rather than recomputed,
		// since `sub/..` resolves back to the directory it started in.
		{"dot in the middle", `printf "[%s]" sub/./*`, `[sub/./f]`},
		{"dot dot in the middle", `printf "[%s]" sub/../*`, `[sub/../a.b][sub/../sub][sub/../vis]`},
		// With nothing behind it, where only a directory survives: `a.b`
		// is a file and `a.b/.` is nothing.
		{"dot is the last component", `printf "[%s]" */.`, `[sub/.]`},
		{"dot dot is the last component", `printf "[%s]" */..`, `[sub/..]`},
		// Quoting a component does not change what it names.
		{"a quoted dot component", `printf "[%s]" "."/sub/*`, `[./sub/f]`},
		{"an escaped dot component", `printf "[%s]" \./sub/*`, `[./sub/f]`},
		// A `.` under a file names nothing, and the miss is reported as any
		// other miss is. Joining without asking what is being joined to
		// would answer this one with a listing of the working directory.
		{"a dot component under a file", `printf "[%s]" a.b/./*`, `[a.b/./*]`},
		{"a dot dot component under a file", `printf "[%s]" a.b/../*`, `[a.b/../*]`},
		{"a dot component over nothing", `printf "[%s]" ./nomatch/*`, `[./nomatch/*]`},
		{"a miss behind a dot component", `printf "[%s]" ./nomatch*`, `[./nomatch*]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runIn(t, dir, tc.src); got != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

// TestADotComponentDoesNotLiftTheLeadingPeriodRule keeps two different rules
// about a period apart, because conflating them would be the serious
// regression here rather than the visible one.
//
// A `.` *component* names a directory. A leading period in a *name* is hidden
// from a pattern that does not write one. So a pattern that begins with a
// period still does not see `.hidden`, and one that writes the period still
// does — measured that way in all six columns.
func TestADotComponentDoesNotLiftTheLeadingPeriodRule(t *testing.T) {
	dir := globDir(t)
	for _, tc := range []struct{ name, src, want string }{
		{"a dot component does not reveal a hidden name", `printf "[%s]" ./*`, `[./a.b][./sub][./vis]`},
		{"a written period still finds one", `printf "[%s]" ./.hi*`, `[./.hidden]`},
		{"and neither does a dot dot component", `printf "[%s]" sub/../*`, `[sub/../a.b][sub/../sub][sub/../vis]`},
		{"which still finds one when written", `printf "[%s]" sub/../.hi*`, `[sub/../.hidden]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runIn(t, dir, tc.src); got != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

// TestAnAbsolutePatternKeepsItsDotComponent is the other branch of the
// render: an absolute pattern strips no prefix, so the spelling survives for
// a different reason and the two need separate evidence.
func TestAnAbsolutePatternKeepsItsDotComponent(t *testing.T) {
	dir := globDir(t)
	for _, tc := range []struct{ name, pattern, want string }{
		{"a dot component", `/./su*`, `[<dir>/./sub]`},
		{"a dot dot component", `/sub/../vi*`, `[<dir>/sub/../vis]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `printf "[%s]" ` + dir + tc.pattern
			want := strings.ReplaceAll(tc.want, "<dir>", dir)
			if got := runIn(t, dir, src); got != want {
				t.Errorf("%s = %s, want %s", src, got, want)
			}
		})
	}
}

// TestABaseWrittenWithATrailingSeparator is the branch a working directory of
// `/` takes, and it is not a hypothetical: `cd /` sets exactly that, and the
// walk then has a base that already ends in a separator.
//
// Two places have to agree about it — the join, which would otherwise build
// `//vis`, and the prefix the render strips, which would otherwise be a
// prefix nothing starts with and leave every match absolute. Measured against
// the panel at the real root: `cd /; printf "[%s]" ./bi*` is `[./bin]` in all
// six, not `[/./bin]`. Pinned in a temporary directory rather than at `/`,
// because what is under the real root is not a fact about this shell.
func TestABaseWrittenWithATrailingSeparator(t *testing.T) {
	dir := globDir(t)
	for _, tc := range []struct{ name, src, want string }{
		{"a plain pattern", `printf "[%s]" v*`, `[vis]`},
		{"a dot component", `printf "[%s]" ./v*`, `[./vis]`},
		{"a dot dot component", `printf "[%s]" sub/../v*`, `[sub/../vis]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runIn(t, dir+"/", tc.src); got != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

// TestADotComponentSurvivesAStarStarDescent is the sibling call site, and it
// is the one a fix to the component walk alone does not reach.
//
// Two helpers build a path: the one that matches a component against a
// listing, and the one that walks everything beneath a directory for `**`.
// A `.` or `..` in front of a `**` is carried by the first and would be
// cleaned away again by the second, so `./**/f` would answer `d/f` where zsh
// answers `./d/f` — the fix present in one helper and missing from its
// neighbor. Mutating only appendDescendants back to a cleaning join survived
// every other test here, which is why this one exists.
func TestADotComponentSurvivesAStarStarDescent(t *testing.T) {
	dir := treeDir(t)
	for _, tc := range []struct{ name, src, want string }{
		{"a leading dot", `echo ./**/f`, "./d/e/f ./d/f ./f"},
		{"a dot in the middle", `echo d/./**/f`, "d/./e/f d/./f"},
		{"a dot dot in the middle", `echo d/../**/f`, "d/../d/e/f d/../d/f d/../f"},
		// The control: without a component to carry, the descent answers
		// what it always answered.
		{"nothing to carry", `echo **/f`, "d/e/f d/f f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opt := withOption(StarStarCrossesDirectories, true,
				withOption(StarStarAloneCrossesDirectories, false, inDir(dir)))
			out, _ := run(t, tc.src, opt)
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
