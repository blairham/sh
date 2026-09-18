// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// A backslash-newline inside the older command substitution is removed before
// the text is parsed, in every quoting written inside it, and the value the
// substitution produces has neither character in it.
//
// Measured 2026-09-16 from script files, `env -i PATH=/usr/bin:/bin LC_ALL=C`:
// bash 5.3, bash 3.2, zsh 5.9.2, ksh93u+ and dash all print `[ab]`, and all
// five count two bytes for the arithmetic row. Before, the pair survived into
// the command text and the substitution produced four characters (#3453).
//
// The `$( )` row is the control and the reason this is not a rule about
// command substitution: the same text in the newer spelling keeps the pair in
// all five columns, so a fix applied to both spellings passes every other
// line here and fails that one.
func TestALineContinuationInsideBackquotesIsRemovedBeforeTheTextIsParsed(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"inside single quotes", "echo \"[`printf %s 'a\\\nb'`]\"", "[ab]"},
		{"inside double quotes", "echo \"[`printf %s \"a\\\nb\"`]\"", "[ab]"},
		{"unquoted in the text", "echo \"[`printf %s a\\\nb`]\"", "[ab]"},
		{"a quoted here-document body", "echo \"[`cat <<'E'\na\\\nb\nE\n`]\"", "[ab]"},
		{"inside arithmetic", "echo \"[$(( `printf %s 'a\\\nb' | wc -c` ))]\"", "[2]"},

		// An escaped backslash is unescaped first, so the newline behind it
		// is an ordinary character: all five print `a`, a newline and `b`.
		{"an escaped backslash", "echo \"[`printf %s 'a\\\\\nb'`]\"", "[a\\\nb]"},

		// The control: the newer spelling hands its program over as written.
		{"the $( ) spelling keeps it", "echo \"[$(printf %s 'a\\\nb')]\"", "[a\\\nb]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src+"\necho after", nil)
			if want := tc.want + "\nafter"; strings.TrimSpace(out) != want {
				t.Errorf("%q printed %q, want %q", tc.src, strings.TrimSpace(out), want)
			}
		})
	}
}
