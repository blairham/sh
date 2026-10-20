// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A builtin or a construct that fills an **array** through a reference asks
// the array literal's question and not the scalar store's: a reference with
// nothing to point at gives the attribute up, and one aimed at an element has
// nowhere to put a container. Measured on bash 5.3.20, 2026-09-22.
func TestAContainerWrittenThroughAReference(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"mapfile over a reference aimed at nothing",
			`typeset -n r; mapfile r < /dev/null 2>/dev/null; typeset -p r`,
			"declare -a r=()\n",
		},
		{
			"and the warning it draws",
			`typeset -n r; mapfile r < /dev/null 2>&1 | sed 's/.*warning/warning/'`,
			"warning: r: removing nameref attribute\n",
		},
		{
			"a coprocess over one fills the name",
			`typeset -n x; { coproc x { :; }; } 2>/dev/null
typeset -p x >/dev/null 2>&1; echo "st=$?"; wait 2>/dev/null`,
			"st=0\n",
		},
		{
			"a coprocess over a reference aimed at an element fills nothing",
			`typeset -n x='A[0]'; { coproc x { :; }; } 2>/dev/null
typeset -p A >/dev/null 2>&1; echo "st=$?"; wait 2>/dev/null`,
			"st=1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := answersRun(t, c.src)
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}

// A reference aimed at an element names one cell, so what is written through
// it is an element of the name the reference wrote down — whoever that name
// turns out to be — and a subscript of its own after it names nothing.
func TestAReferenceAimedAtAnElement(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the base gives its own reference up",
			`typeset -n a=v; typeset -n b='a[1]'; b=foo; typeset -p a`,
			"sh: line 1: warning: a: removing nameref attribute\n" +
				"declare -a a=([1]=\"foo\")\n",
		},
		{
			"so the far name is never written",
			`typeset -n a=v; typeset -n b='a[1]'; b=foo 2>/dev/null
typeset -p v >/dev/null 2>&1; echo "st=$?"`,
			"sh: line 1: warning: a: removing nameref attribute\nst=1\n",
		},
		{
			"and round a cycle it is the same",
			`typeset -n a=b; typeset -n b='a[1]'; a=foo; typeset -p a`,
			"sh: line 1: warning: a: removing nameref attribute\n" +
				"declare -a a=([1]=\"foo\")\n",
		},
		{
			"where the direct spelling still goes through the reference",
			`typeset -n a=v; a[1]=foo; typeset -p v`,
			"declare -a v=([1]=\"foo\")\n",
		},
		{
			"a subscript of its own is refused",
			`typeset -n r='A[0]'; r[1]=bar`,
			"sh: line 1: `A[0]': not a valid identifier\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := answersRun(t, c.src)
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}
