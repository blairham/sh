// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// TestAGroupedUnaryStandingAloneNeverCloses: this shell refuses a group that
// holds a unary operator and its operand and nothing else, as a closing
// parenthesis its reading never reached — where every other column in the
// panel evaluates the group and answers 0 (#3419).
//
// The controls are the finding and not decoration: a parenthesized *string*, a
// parenthesized comparison, a `!` inside the group and the same group with
// anything at all behind it are each read. So this is the one shape rather
// than "no grouping here", which is what a fix aimed at the parentheses would
// have made of it.
//
// Measured 2026-09-18 on BusyBox v1.37.0 in the digest-pinned image, each
// snippet a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin
// on /dev/null.
func TestAGroupedUnaryStandingAloneNeverCloses(t *testing.T) {
	for _, tc := range []struct {
		src    string
		want   string
		status int
	}{
		{`[ \( -n x \) ]`, "closing paren expected", 2},
		{`[ \( -z "" \) ]`, "closing paren expected", 2},
		{`[ \( -n \) ]`, "closing paren expected", 2},
		{`[ ! \( -n x \) ]`, "closing paren expected", 2},
		{`[[ \( -n x \) ]]`, "closing paren expected", 2},
		{`[ \( ! -n x \) ]; echo "st=$?"`, "st=1", 0},
		{`[ \( ! x \) ]; echo "st=$?"`, "st=1", 0},
		{`[ \( a \) ]; echo "st=$?"`, "st=0", 0},
		{`[ \( a = a \) ]; echo "st=$?"`, "st=0", 0},
		{`[ \( \( -n x \) \) ]; echo "st=$?"`, "st=0", 0},
		{`[ \( -n x \) -a x ]; echo "st=$?"`, "st=0", 0},
		{`[ x -a \( -n x \) ]; echo "st=$?"`, "st=0", 0},
		{`[ \( -n x -a y \) ]; echo "st=$?"`, "st=0", 0},
	} {
		out, st := run(t, tc.src)
		if !strings.Contains(out, tc.want) || st != tc.status {
			t.Errorf("%s wrote %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}

// TestAnUnmatchedBracketMatchesItself: a `[` no `]` closes is an ordinary
// character in a pattern here, which is bash's and ksh93's reading and not the
// sibling's — the value this dialect had been given without measuring it
// (#3420).
//
// The last two rows are the sub-expression axis, which #3379 left open for
// want of a BusyBox to ask: a `[:name:]` inside the bracket does not close it
// either, and the reading is the same literal one. They are written as a
// prefix trim so that what matched is visible as text rather than as a status.
func TestAnUnmatchedBracketMatchesItself(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
	}{
		{`case [ in [) echo hit;; *) echo miss;; esac`, "hit"},
		{`case a[ in a[) echo hit;; *) echo miss;; esac`, "hit"},
		{`v='['; case [ in $v) echo hit;; *) echo miss;; esac`, "hit"},
		{`case '[' in '[') echo hit;; *) echo miss;; esac`, "hit"},
		{`[[ "[" == "[" ]]; echo "st=$?"`, "st=0"},
		{`w='[a'; echo "[${w#[}]"`, "[a]"},
		{`w='[a'; echo "[${w#[[:alpha:]}]"`, "[]"},
	} {
		if out, _ := run(t, tc.src); strings.TrimSpace(out) != tc.want {
			t.Errorf("%s wrote %q, want %q", tc.src, out, tc.want)
		}
	}
}

// TestALocalThroughCommandDeclaresNothing: the prefix runs the word for its
// operand checks and puts the declaration nowhere — nothing declared, nothing
// assigned, 0 — where bash declares the local (#3370).
//
// The bad-name row is what says the builtin still runs: this shell gives
// `local` no option letters, so `-x` is a name, and the refusal is the one a
// bare `local` gives for it. The export and readonly rows say it is `local`
// alone.
func TestALocalThroughCommandDeclaresNothing(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
	}{
		{`f() { command local a=1; echo "[${a-UNSET}]"; }; a=outer; f; echo "[$a]"`, "[outer]\n[outer]"},
		{`f() { command local b; echo "[${b-UNSET}] st=$?"; }; f`, "[UNSET] st=0"},
		{`command local a=1; echo "[${a-UNSET}] st=$?"`, "[UNSET] st=0"},
		{`f() { local c=2; echo "[$c]"; }; f`, "[2]"},
		{`f() { command export d=1; echo "[$d]"; }; f`, "[1]"},
		{`f() { command readonly e=1; echo "[$e]"; }; f`, "[1]"},
	} {
		if out, _ := run(t, tc.src); strings.TrimSpace(out) != tc.want {
			t.Errorf("%s wrote %q, want %q", tc.src, out, tc.want)
		}
	}
	out, st := run(t, `f() { command local -x c=1; }; f`)
	if !strings.Contains(out, "local") || !strings.Contains(out, "-x: bad variable name") || st != 2 {
		t.Errorf("a bad name under the prefix wrote %q at %d, want the builtin's own refusal at 2", out, st)
	}
}
