// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// An unescaped `&` in a `${v/pat/rep}` replacement is the text the pattern
// matched here, and `shopt patsub_replacement` is on with nothing said.
//
// Measured against bash 5.3.15 on 2026-09-11 with `v=abc`. It is this shell
// alone in the panel: bash 3.2.57, ksh93 and zsh 5.9.2 all answer `a[&]c` for
// the first row, and bash 3.2 does not have the option name at all.
func TestPatsubReplacementReadsTheAmpersandByDefault(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v=abc; echo "${v/b/[&]}"`, "a[b]c"},
		{`v=abc; echo "${v//b/[&]}"`, "a[b]c"},
		{`v=abcabc; echo "${v//b/<&>}"`, "a<b>ca<b>c"},
		{`v=abcabc; echo "${v//[ab]/<&>}"`, "<a><b>c<a><b>c"},
		{`v=aXbXc; echo "${v/X*X/<&>}"`, "a<XbX>c"},
		{`v=hello; echo "${v//l/&&}"`, "hellllo"},
		{`v=abcabc; echo "${v/#a/<&>}"`, "<a>bcabc"},
		{`v=abcabc; echo "${v/%c/<&>}"`, "abcab<c>"},
		// Quoting is what makes an `&` ordinary, and the enclosing quotes are
		// not what is asked.
		{`v=abc; echo "${v/b/[\&]}"`, "a[&]c"},
		{`v=abc; echo "${v/b/"&"}"`, "a&c"},
		{`v=abc; echo "${v/b/'&'}"`, "a&c"},
		{`v=abc; echo "${v/b/$'&'}"`, "a&c"},
		// A replacement that arrived from an expansion is read, and its
		// quoting counts the same way.
		{`v=abc; r='&'; echo "${v/b/$r}"`, "abc"},
		{`v=abc; r='&'; echo "${v/b/"$r"}"`, "a&c"},
		{`v=abc; r='[&]'; echo "${v/b/$r}"`, "a[b]c"},
		// The escape half, in the text an expansion brought.
		{`v=abc; r='[\&]'; echo "${v/b/$r}"`, "a[&]c"},
		{`v=abc; r='[\\&]'; echo "${v/b/$r}"`, `a[\b]c`},
		{`v=abc; r='[\a]'; echo "${v/b/$r}"`, `a[\a]c`},
		// A written backslash is removed by ordinary quote removal before the
		// replacement is read, which is why these two agree in both states.
		{`v=abc; echo "${v/b/[\a&]}"`, "a[ab]c"},
		{`v=abc; echo "${v/b/[\\&]}"`, `a[\b]c`},
		// Nothing matched, so the replacement is never read.
		{`v=abc; echo "${v/zzz/<&>}"`, "abc"},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", tc.src, got, st, tc.want)
		}
	}
}

// `shopt -u patsub_replacement` turns the reading back off, and `shopt -s`
// turns it on again — which is the half that says this is an option and not a
// dialect's fixed answer.
func TestPatsubReplacementTurnsOffAndOnAgain(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v=abc; shopt -u patsub_replacement; echo "${v/b/[&]}"`, "a[&]c"},
		{`v=abcabc; shopt -u patsub_replacement; echo "${v//b/<&>}"`, "a<&>ca<&>c"},
		{
			`v=abc; shopt -u patsub_replacement; shopt -s patsub_replacement; echo "${v/b/[&]}"`,
			"a[b]c",
		},
		// The escape half moved with the reading: with it on this is `a[&]c`.
		{`v=abc; r='[\&]'; shopt -u patsub_replacement; echo "${v/b/$r}"`, `a[\&]c`},
		// And this one did not, because that backslash never reaches the
		// replacement reader.
		{`v=abc; shopt -u patsub_replacement; echo "${v/b/[\&]}"`, "a[&]c"},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", tc.src, got, st, tc.want)
		}
	}
}

// The name reports **on**, which is the whole of #1712's deferred half: it was
// held at off while the behavior it gates was unbuilt, so a real bash's own
// `shopt -p` sourced into this shell wrote a `not implemented` line for it.
func TestPatsubReplacementReportsOn(t *testing.T) {
	for _, tc := range []struct {
		src    string
		want   string
		status int
	}{
		{`shopt -p patsub_replacement`, "shopt -s patsub_replacement\n", 0},
		{`shopt patsub_replacement`, "patsub_replacement  \ton\n", 0},
		{`shopt -q patsub_replacement`, "", 0},
		{`shopt -u patsub_replacement; shopt -p patsub_replacement`, "shopt -u patsub_replacement\n", 1},
		// The line a real bash writes, read back without a complaint — which
		// it was not while the name sat in the refusing table.
		{`shopt -s patsub_replacement; echo ok`, "ok\n", 0},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s = %q status %d, want %q status %d",
				tc.src, out, st, tc.want, tc.status)
		}
	}
}

// A subshell's `shopt -u` never escapes, the same bargain every other wired
// name strikes.
func TestPatsubReplacementStaysInsideASubshell(t *testing.T) {
	out, _ := runBash(t, t.TempDir(),
		`v=abc; (shopt -u patsub_replacement); echo "${v/b/[&]}"`)
	if got := strings.TrimSpace(out); got != "a[b]c" {
		t.Errorf("a subshell's shopt escaped: %q", got)
	}
}
