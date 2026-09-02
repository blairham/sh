// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
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
