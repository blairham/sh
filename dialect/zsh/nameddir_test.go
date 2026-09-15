// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `hash -d` is this dialect's named-directory table, end to end, and `~name`
// reads it back.
//
// Measured on zsh 5.9.2, 2026-09-14. This dialect answered `bad option: -d`
// and left `~name` as written (#2191), so the whole construct was missing —
// both halves of it, since the table is what the tilde reads.
//
// Nothing here reads the machine's users: a named directory is a table this
// shell owns outright and wants nothing from the operating system. The user
// database's half is a hook the binary wires, and is pinned in driver and in
// interp against a table written in the test.
func TestANamedDirectoryIsWrittenByHashAndReadByATilde(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"written and read back", `hash -d a=/tmp; print -r -- ~a`, "/tmp\n"},
		{"with a tail behind it", `hash -d a=/tmp; print -r -- ~a/x`, "/tmp/x\n"},
		{"a trailing slash is a tail too", `hash -d a=/tmp; print -r -- ~a/`, "/tmp/\n"},
		{"quoted, it is text", `hash -d a=/tmp; print -r -- "~a"`, "~a\n"},
		{"not at the head of a word, it is text", `hash -d a=/tmp; print -r -- x~a`, "x~a\n"},
		{"a name it has not got is text", `hash -d a=/tmp; print -r -- ~b`, "~b\n"},
		// The listing, sorted by name rather than by the order the entries
		// were written.
		{"the listing is sorted", `hash -d z=/z b=/b a=/a; hash -d`, "a=/a\nb=/b\nz=/z\n"},
		{"and `-L` writes the command back", `hash -d a=/tmp; hash -dL`, "hash -d a=/tmp\n"},
		{"an empty table lists nothing", `hash -d`, ""},
		{"`-r` empties it", `hash -d a=/tmp; hash -d -r; hash -d; echo done`, "done\n"},
		// A value is taken as written: not resolved, not checked.
		{"a relative value stands", `hash -d a=rel; print -r -- ~a`, "rel\n"},
		{"an empty one is visible in the listing", `hash -d a=; hash -d`, "a=''\n"},
		// The names are wider than a shell identifier.
		{"a name may begin with a digit", `hash -d 2a=/tmp; print -r -- ~2a`, "/tmp\n"},
		{"and hold a dash", `hash -d a-b=/tmp; print -r -- ~a-b`, "/tmp\n"},
		{"and a dot", `hash -d a.b=/tmp; print -r -- ~a.b`, "/tmp\n"},
		// It reaches a pattern as well as a word, which is the half #2181
		// settled for the bare tilde and which this inherits.
		{"a condition reads it", `hash -d a=/tmp; [[ /tmp/x == ~a/* ]] && echo Y`, "Y\n"},
		{"and a case arm", `hash -d a=/tmp; case /tmp in ~a) echo Y;; *) echo N;; esac`, "Y\n"},
		// And an assignment's value, where a tilde expands too.
		{"an assignment reads it", `hash -d a=/tmp; x=~a; print -r -- $x`, "/tmp\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
	// The two complaints, in this shell's own words.
	for _, tc := range []struct{ name, src, want string }{
		{"a name the table has not got", `hash -d nosuch`, "no such directory name: nosuch"},
		{"a name a tilde could never read back", `hash -d a/b=/tmp`, "invalid character in directory name: a/b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) || st != 1 {
				t.Errorf("%s gave %q at %d, want %q at 1", tc.src, out, st, tc.want)
			}
		})
	}
}
