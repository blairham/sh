// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Expand turns a setting into what it names.
//
// For a caller holding `ENV=$HOME/.shrc` and having to open it: without this
// it would have to parse and expand, which is the whole of this package.
func TestExpandASetting(t *testing.T) {
	sem := permissive()
	r := &Runner{Semantics: &sem, Vars: map[string]string{
		"HOME": "/home/someone",
		"NAME": "rc",
	}}
	for _, c := range []struct{ in, want string }{
		{"$HOME/.shrc", "/home/someone/.shrc"},
		{"${HOME}/.shrc", "/home/someone/.shrc"},
		{"/etc/shrc", "/etc/shrc"},
		{"$HOME/.$NAME", "/home/someone/.rc"},
		{"${MISSING:-/fallback}", "/fallback"},
		{"", ""},
		// No field splitting: a setting that names a file names one, and a
		// path with a space in it is still one path.
		{"$HOME/my file", "/home/someone/my file"},
	} {
		if got := r.Expand(c.in); got != c.want {
			t.Errorf("%q gave %q, want %q", c.in, got, c.want)
		}
	}
}

// TestATildeAfterAColonInAnAssignment — the assignment context adds a tilde
// after each unquoted colon, with the same limits as the leading one: quoting
// turns it off, a user name is left for a database this package does not
// carry, and a segment that runs into an expansion keeps its tilde.
func TestATildeAfterAColonInAnAssignment(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"expands after each colon", `v=a:~/b:~; echo "$v"`, "a:/home/who/b:/home/who"},
		{"leading and colon together", `v=~/x:~/y; echo "$v"`, "/home/who/x:/home/who/y"},
		{"quoted stays literal", `v=":~/q"; echo "$v"`, ":~/q"},
		{"a user name is left alone", `v=a:~nobody/x; echo "$v"`, "a:~nobody/x"},
		{"a segment into an expansion keeps its tilde", `u=/x; v=a:~$u; echo "$v"`, "a:~/x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, func(r *Runner) {
				r.Vars = map[string]string{"HOME": "/home/who"}
			})
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}
