// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A `[` behind an unbraced name whose `]` was written inside a command
// substitution: no subscript, and the bracket run is kept **as written**.
//
// Measured on zsh 5.9.2, 2026-09-14, `setopt noglob` so a pattern matching
// nothing cannot be what is reported, with `a=(xx yy zz)`. This shell ran the
// substitution and spliced its output in — `zz[2]` — which is neither what
// zsh does nor what the shells with no bare subscript do (#2786).
//
// `printf '<%s>'` rather than a status check, because both columns are at 0
// and the whole of the difference is which characters come out.
func TestABracketRunABareSubscriptGaveBackIsKeptAsWritten(t *testing.T) {
	const arr = "setopt noglob; a=(xx yy zz); "
	for _, tc := range []struct{ name, src, want string }{
		{"the `]` is inside the substitution", `printf "<%s>" $a[$(: ]; echo 2)]`, "<xx><yy><zz[$(: ]; echo 2)]>"},
		{"the `[` inside it is never closed", `printf "<%s>" $a[$(echo 2; : [)]`, "<xx><yy><zz[$(echo 2; : [)]>"},
		{"a parameter between the brackets is kept too", `v=V; printf "<%s>" $a[$(: ]; echo $v)]`, "<xx><yy><zz[$(: ]; echo $v)]>"},
		// The blanks between the brackets do not split a field and the `*`
		// there is not a pattern, which is what says the run is quoted
		// rather than merely put back.
		{"a `*` between the brackets is a character", `printf "<%s>" $a[$(: ]; echo *)]`, "<xx><yy><zz[$(: ]; echo *)]>"},
		// And only the run is quoted. What stands behind the closing bracket
		// is a word like any other.
		{"a substitution behind it still runs", `printf "<%s>" $a[$(: ]; echo 2)]$(echo Q)`, "<xx><yy><zz[$(: ]; echo 2)]Q>"},
		{"and a parameter behind it", `v=V; printf "<%s>" $a[$(: ]; echo 2)]$v`, "<xx><yy><zz[$(: ]; echo 2)]V>"},
		// The extent is the `]` that closes the `[` with the `$( )` stepped
		// over whole, so a second one behind it is ordinary text.
		{"a second `]` behind it is text", `printf "<%s>" $a[$(: ]; echo 2)]]`, "<xx><yy><zz[$(: ]; echo 2)]]>"},
		// The control: the same shape with the bracket closed *outside* the
		// substitution is a subscript, and is read.
		{"a bracket the substitution closes is a subscript", `printf "<%s>" $a[$(echo 2; : [ ])]`, "<yy>"},
		{"and so is a plain one", `printf "<%s>" $a[$(echo 2)]`, "<yy>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, arr+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// And a `]` that closes nothing once the substitution is stepped over whole
// leaves the subscript unfinished, which is a refusal rather than a word.
//
// It is the control that keeps the test above from reading as "any `]`
// anywhere in the word closes it" — the reading this shell had, which is why
// `$a[$(: ]; echo 2)` was expanded rather than refused. The backtick row is
// where the two spellings of a substitution part: measured, zsh refuses
// `$a[`: ]; echo 2`]` while keeping the `$( )` spelling of the same word.
func TestABareSubscriptClosedOnlyInsideASubstitutionIsRefused(t *testing.T) {
	const arr = "setopt noglob; a=(xx yy zz); "
	for _, tc := range []struct{ name, src string }{
		{"nothing after the substitution", `printf "<%s>" $a[$(: ]; echo 2)`},
		{"text after it", `printf "<%s>" $a[$(: ]; echo 2)x`},
		{"a backtick holding the only `]`", "printf \"<%s>\" $a[`: ]; echo 2`]"},
		{"a backtick whose `]` is quoted inside it", "printf \"<%s>\" $a[`echo \"]\"`]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, arr+tc.src)
			if !strings.Contains(out, "invalid subscript") || st == 0 {
				t.Errorf("%s gave %q at %d, want the subscript refused", tc.src, out, st)
			}
		})
	}
	// A backtick *inside* a `$( )` is no different from any other character
	// there, because the group around it is stepped over whole.
	out, st := answersRun(t, arr+"printf \"<%s>\" $a[$(: ]; echo `echo 2`)]")
	if want := "<xx><yy><zz[$(: ]; echo `echo 2`)]>"; out != want || st != 0 {
		t.Errorf("gave %q at %d, want %q at 0", out, st, want)
	}
}
