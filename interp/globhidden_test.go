// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"testing"
)

// hiddenDir is one hidden name and one plain one, which is the whole of what
// the leading-period rule is about.
func hiddenDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{".hidden", "plain"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// A pattern matches a leading period only if it explicitly begins with one —
// and where it begins with a group, every alternative of that group is a
// place it could begin.
//
// Every row is a measurement on zsh 5.9.2 with `extendedglob` on, in exactly
// the directory hiddenDir builds. See interp/globhidden.go for the reasoning
// and for what reads only the pattern's first byte gets wrong.
func TestAGroupsAlternativesCanBeginWithAPeriod(t *testing.T) {
	dir := hiddenDir(t)
	for _, tc := range []struct{ name, src, want string }{
		{"a plain period", `printf "[%s]" .hidden(#qN)`, "[.hidden]"},
		{"an escaped one", `printf "[%s]" \.hidden(#qN)`, "[.hidden]"},
		{"a period in front of a group", `printf "[%s]" .(hidden|x)(#qN)`, "[.hidden]"},
		{"the first alternative of a group", `printf "[%s]" (.hidden|plain)(#qN)`, "[.hidden][plain]"},
		{"and a later one", `printf "[%s]" (x|.hidden)(#qN)`, "[.hidden]"},
		{"an alternative that is only the period", `printf "[%s]" (.|x)hidden(#qN)`, "[.hidden]"},
		{"a group of one", `printf "[%s]" (.hidden)(#qN)`, "[.hidden]"},
		{"a group inside a group", `printf "[%s]" ((.hidden))(#qN)`, "[.hidden]"},
		{"an alternative that goes on to be a pattern", `printf "[%s]" (.h*)(#qN)`, "[.hidden]"},
		{"past an empty alternative", `printf "[%s]" (|.hidden)(#qN)`, "[.hidden]"},
		{"past a pattern-flag group", `printf "[%s]" (#i)(.HIDDEN|x)(#qN)`, "[.hidden]"},
		{"with more pattern after the group", `printf "[%s]" (.hidden|plain)*(#qN)`, "[.hidden][plain]"},
		// The three that say this is "is one written there" rather than
		// "could this pattern match one".
		{"a bracket is not explicit", `printf "[%s]" [.]hidden(#qN)`, "[]"},
		{"nor is a metacharacter", `printf "[%s]" ?hidden(#qN)`, "[]"},
		{"which is the rule's whole point", `printf "[%s]" *(#qN)`, "[plain]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runCondition(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
