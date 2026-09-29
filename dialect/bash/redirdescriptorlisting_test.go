// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// What `declare -f` does with a descriptor that is the operator's default,
// which is **two opposite answers in one shell**.
//
// A duplication gets the default written in — `>&2` comes back `1>&2` — and
// a redirection that names a file gets it taken off — `1>out` comes back
// `> out`. That pair is why syntax.Layout.RedirectDescriptor is a named form
// rather than a "normalize descriptors" bool: no single verb covers this
// column.
//
// The file half was wrong here until #4436 and is the second row below: this
// shell was saying `1> out` where bash 5.3.20 says `> out`. It was found
// while fixing zsh's listing, because one printer serves all three columns
// and it had one unconditional rule measured on this one.
//
// Measured 2026-09-29 on bash 5.3.20, `f() { … }; declare -f f`.
func TestDeclareWritesTheDefaultOnDuplicationsAndDropsItFromFiles(t *testing.T) {
	for _, tc := range []struct{ name, written, want string }{
		{"a duplication gets it written in", "echo x >&2", "echo x 1>&2"},
		{"a file redirection loses it", "echo x 1>out", "echo x > out"},
		{"a read duplication likewise", "read v <&3", "read v 0<&3"},
		{"one already there stays", "echo x 2>&1", "echo x 2>&1"},
		{"a close carries it too", "echo x >&-", "echo x 1>&-"},
		{"and a close is written one way whichever asked", "read v <&-", "read v 0>&-"},
		{"a file redirection that named another keeps it", "echo x 2>out", "echo x 2> out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs := runBashSplit(t, "f() { "+tc.written+"; }\ndeclare -f f\n")
			if errs != "" {
				t.Fatalf("stderr %q", errs)
			}
			if !containsLine(out, tc.want) {
				t.Errorf("listing %q, want a line %q", out, tc.want)
			}
		})
	}
}

// containsLine reports whether some line of the listing, with its
// indentation taken off, is exactly want. The listing's indentation is this
// shell's own and is not what these rows are about.
func containsLine(out, want string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}
