// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// A parameter `zmodload -F` leaves off stops answering (#1841).
//
// The builtin half of this is #1635, and the two need different seams because
// this shell answers differently: a deselected builtin refuses at 127, a
// deselected parameter says nothing at all. Measured on zsh 5.9.2, 2026-09-10,
// with one function defined and `zmodload -F zsh/parameter -p:functions`:
// `${#functions}` is 0, `${+functions}` is 0, `${functions[f]}` is empty and
// `${(k)functions}` is empty, all at status 0.
//
// Ours recorded the selection, reported it truthfully through `-lF`, and went
// on answering with the real count.
func TestADeselectedParameterStopsAnswering(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// Every reading at once, because they reach the tables by
			// different routes and a seam that gated one and missed another
			// would answer half of this.
			"every reading goes to the unset answer together",
			`zmodload zsh/parameter
f() { :; }
print "before=${#functions}"
zmodload -F zsh/parameter -p:functions
print "n=${#functions} set=${+functions} elem=[${functions[f]}] keys=[${(k)functions}]"
`,
			"before=1\nn=0 set=0 elem=[] keys=[]\n",
		},
		{
			"and the sign puts it back",
			`zmodload zsh/parameter
f() { :; }
zmodload -F zsh/parameter -p:functions
zmodload -F zsh/parameter +p:functions
print "back=${#functions}"
`,
			"back=1\n",
		},
		{
			// A plain load of the module widens it again, which is the other
			// way back and the one a script is likelier to take.
			"and so does loading the module again",
			`zmodload zsh/parameter
f() { :; }
zmodload -F zsh/parameter -p:functions
zmodload zsh/parameter
print "reload=${#functions}"
`,
			"reload=1\n",
		},
		{
			// The name is *ordinary* while it is off, not read-only with
			// nothing in it. `$parameters` carries the mark so that a write
			// cannot shadow its own producer; with the producer gone there is
			// nothing to shadow. Measured: real zsh assigns and prints `a b`.
			"a deselected parameter is an ordinary name",
			`zmodload zsh/parameter
zmodload -F zsh/parameter -p:parameters
parameters=(a b)
print "assigned=[$parameters] st=$?"
`,
			"assigned=[a b] st=0\n",
		},
		{
			// The type word is the one-name spelling of `$parameters`, and it
			// has to agree: a withdrawn name is not a parameter this shell
			// has, so it says what it says for any unset name.
			"and the type word agrees",
			`zmodload zsh/parameter
zmodload -F zsh/parameter -p:functions
print "type=[${(t)functions}]"
`,
			"type=[]\n",
		},
		{
			// The selection is state on the runner, so a subshell's is the
			// subshell's — the same rule the builtin half keeps.
			"a selection made in a subshell stays there",
			`zmodload zsh/parameter
f() { :; }
(zmodload -F zsh/parameter -p:functions; print "in=${#functions}")
print "after=${#functions}"
`,
			"in=0\nafter=1\n",
		},
		{
			// And the listing still reports it, which it did before this
			// change and has to go on doing: recording the selection was
			// never the part that was missing.
			"the listing reports it either way",
			`zmodload zsh/parameter
zmodload -F zsh/parameter -p:functions
rows=$(zmodload -lF zsh/parameter)
case $rows in (*"-p:functions"*) print listed=off ;; (*) print listed=on ;; esac
`,
			"listed=off\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
