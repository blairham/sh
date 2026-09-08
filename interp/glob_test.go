// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
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

// A substituted word's quoting survives it, and the match that word takes
// part in is the *enclosing* word's.
//
// Two halves of one fault, and it is one fault because both come of expanding
// the operand through the entry point that finishes a whole word: it matched
// what it had in hand and handed back the unescaped result, so the quoting
// inside it was gone before anything else could read it.
//
// Measured 2026-09-08 across bash 5.3.15, that build as `sh`, bash 3.2.57,
// dash, ksh93 and zsh 5.9.2, in a directory holding `Xay` and `Xby`. All six
// agree on every row:
//
//	${u:-"X[a-b]y"}    X[a-b]y     quoted: seven characters, not a pattern
//	${u:+"X[a-b]y"}    X[a-b]y     and `+` is the same operand rule
//	${u-"X[a-b]y"}     X[a-b]y     and so is the colonless spelling
//	X${u:-[a-b]}y      Xay Xby     unquoted: the whole word is the pattern
//
// The last row is the one the operand-alone match could never give: it found
// no file named `[a-b]`, so a shell whose unmatched-pattern rule is fatal
// stopped the command there.
//
// This is the shape a real `zi.zsh` hits at its color table, which is why it
// is worth a test rather than only a corpus row. That file chooses its
// separator characters by testing `$LANG` itself, in the shape
//
//	${${${(M)LANG:#*UTF-8*}:+…}:-…}
//
// with a `$'…'` color sequence on each side — so a UTF-8 locale takes the
// `+` operand of the *inner* expansion, whose fields the outer one then
// carries, and the C locale takes the `-` operand of the outer one, which is
// the only side nothing wrapped. A color sequence opens `ESC [`, so that
// operand is an unterminated bracket expression once its quoting is lost,
// and zsh's answer to that is fatal: `bad pattern` at the line, under
// `LANG=C` and not under `LANG=en_US.UTF-8` (#1500).
func TestASubstitutedWordKeepsItsQuotingAndIsMatchedWithTheWordAroundIt(t *testing.T) {
	dir := globDir(t)
	for _, tc := range []struct{ name, src, want string }{
		{"a quoted operand is text", `unset u; printf "[%s]" ${u:-"a.[a-c]"}`, `[a.[a-c]]`},
		{"and `+` is the same rule", `u=set; printf "[%s]" ${u:+"a.[a-c]"}`, `[a.[a-c]]`},
		{"and so is the colonless spelling", `unset u; printf "[%s]" ${u-"a.[a-c]"}`, `[a.[a-c]]`},
		{"a single-quoted operand too", `unset u; printf "[%s]" ${u:-'a.[a-c]'}`, `[a.[a-c]]`},
		// The unquoted operand still matches, and it matches as part of the
		// word it sits in rather than on its own.
		{"an unquoted operand is a pattern", `unset u; printf "[%s]" ${u:-a.[a-c]}`, `[a.b]`},
		{"matched with the word around it", `unset u; printf "[%s]" a.${u:-[a-c]}`, `[a.b]`},
		{"and the text around it counts", `unset u; printf "[%s]" q.${u:-[a-c]}`, `[q.[a-c]]`},
		// Half quoted and half not is the discriminating shape: one field,
		// with a live metacharacter beside a marked one.
		{"quoting is per span, not per operand", `unset u; printf "[%s]" ${u:-"a."[a-c]}`, `[a.b]`},
		{"the other way round", `unset u; printf "[%s]" ${u:-a."[a-c]"}`, `[a.[a-c]]`},
		// The braces are the operand's own stage and they run before the
		// match, so a braced operand keeps its quoting through both. It is
		// the row that says the brace branch and the plain one answer the
		// same question — a fix applied to one of them and not the other
		// would pass every row above (zsh 5.9.2 gives the same two fields).
		{"a braced operand keeps its quoting", `unset u; printf "[%s]" ${u:-{a,q}."[a-c]"}`, `[a.[a-c]][q.[a-c]]`},
		{"and an unquoted one still matches", `unset u; printf "[%s]" ${u:-{a,q}.[a-c]}`, `[a.b][q.[a-c]]`},
		// A backslash in the operand is text and stays in the text, which
		// is #1222's rule reaching this path: dropping the marks dropped
		// it with them, so `v=${u:-"a\b"}` assigned `ab` where all six
		// assign three characters (measured 2026-09-08).
		{"a backslash in the operand survives", `unset u; v=${u:-"a\b"}; printf "[%s]" "$v"`, `[a\b]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runIn(t, dir, tc.src); got != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
}

// The escape sequence that found it, in the form a startup file writes one.
//
// `$'…'` is a quoting form, so its text is never a pattern — and every color
// sequence opens with `ESC [`, which is an unterminated bracket expression
// the moment that stops being true. Separate from the rows above because it
// needs a grammar the core does not carry, and because the byte rather than
// the bracket is what a reader of #1500 will come looking for.
func TestAnEscapeSequenceInASubstitutedWordIsText(t *testing.T) {
	dir := globDir(t)
	dollarSingle := func(d *syntax.Dialect) { d.DollarSingleQuote = true }
	for _, tc := range []struct{ name, src, want string }{
		{
			"the operand of `-`", `unset u; printf "[%s]" ${u:-$'\e[38;5;82m«-»\e[0m'}`,
			"[\x1b[38;5;82m«-»\x1b[0m]",
		},
		{"the operand of `+`", `u=set; printf "[%s]" ${u:+$'\e[1m'}`, "[\x1b[1m]"},
		{"and the value it lands in", `unset u; v=${u:-$'\e[0m'}; printf "[%s]" "$v"`, "[\x1b[0m]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := runGrammar(t, tc.src, dollarSingle, func(r *Runner) { r.Dir = dir })
			if got != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, st, tc.want)
			}
		})
	}
}

// The assigning operators store the word, not what it would have matched.
//
// `${u:=word}` and `${u::=word}` substitute through the parameter, and the
// word reaches it as text: measured 2026-09-08 in a directory holding `Xay`
// and `Xby`, `u=; printf "<%s>" "${u:=X[a-b]y}"` leaves `X[a-b]y` in the
// parameter in all six of bash 5.3.15, that build as `sh`, bash 3.2.57, dash,
// ksh93 and zsh 5.9.2. Matching on the way in stored the listing instead —
// `Xay Xby`, in the variable, where every one of them keeps the seven
// characters (#1500).
//
// What the *expansion* then comes to is a different question with a different
// answer, and it is GlobExpansionResults: the five read the stored value
// back as a pattern and zsh does not. That axis is asked elsewhere; this test
// reads the parameter through quotes so it does not depend on it.
func TestAnAssigningOperatorStoresTheWordUnmatched(t *testing.T) {
	dir := globDir(t)
	for _, tc := range []struct{ name, src, want string }{
		{"`:=` stores the text", `unset u; : ${u:=a.[a-c]}; printf "[%s]" "$u"`, `[a.[a-c]]`},
		{"a quoted word likewise", `unset u; : ${u:="a.[a-c]"}; printf "[%s]" "$u"`, `[a.[a-c]]`},
		{"and a plain word is unaffected", `unset u; : ${u:=vis}; printf "[%s]" "$u"`, `[vis]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runIn(t, dir, tc.src); got != tc.want {
				t.Errorf("%s = %s, want %s", tc.src, got, tc.want)
			}
		})
	}
	// `::=` is the same rule from the operator that asks nothing, and it is
	// a separate call rather than a row above because it needs a grammar
	// the core does not carry. Without it a mutant reverting only the
	// unconditional half survives every row in this file.
	//
	// The parameter is read through quotes here for the reason the rows
	// above are: what the *expansion* comes to is GlobExpansionResults',
	// which a synthetic core does not answer the way zsh does, and this
	// case is about what the assignment stored.
	t.Run("and the unconditional spelling", func(t *testing.T) {
		src := `u=old; : ${u::=a.[a-c]}; printf "[%s]" "$u"`
		got, st := runGrammar(t, src, func(d *syntax.Dialect) { d.ParamAssignAlways = true },
			func(r *Runner) { r.Dir = dir })
		if want := `[a.[a-c]]`; got != want || st != 0 {
			t.Errorf("%s = %s (status %d), want %s at 0", src, got, st, want)
		}
	})
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
