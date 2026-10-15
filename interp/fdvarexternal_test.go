// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A `{name}` redirection on a command that is a program of its own is made in
// that program's process, so the shell keeps neither the name nor the
// descriptor — and the command's own later redirections still read the name,
// because they are made in the same process. The builtin and function rows
// are the controls: the same redirection on a command the shell runs itself
// leaves the number behind. See interp/fdvarexternal.go.
//
// `cat` is the program because the core has no builtin of that name, so it
// is found on PATH. Every row answers the outliving axis Yes, which is the
// answer that would keep the descriptor if anything did: what is taken back
// here is taken back because of the process, not because of the axis.
func TestAnFdVariableOnAProgramIsThatProgramsOwn(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{"the name stays unset", `cat {v}</dev/null </dev/null; echo "[${v-unset}]"`, "[unset]\n"},
		{"the name keeps its value", `v=keep; cat {v}</dev/null </dev/null; echo "[$v]"`, "[keep]\n"},
		{
			"two stores to one name are both taken back",
			`v=keep; cat {v}</dev/null {v}</dev/null </dev/null; echo "[$v]"`,
			"[keep]\n",
		},
		{
			"the descriptor is not kept",
			`exec {x}</dev/null; exec {x}<&-
cat {v}</dev/null </dev/null
exec {y}</dev/null
[ "$x" = "$y" ] && echo reused`,
			"reused\n",
		},
		{
			"a later redirection of the same command reads the name",
			`echo data >src; cat {v}<src 0<&$v; echo "[${v-unset}]"`,
			"data\n[unset]\n",
		},
		{"a builtin keeps the name", `: {v}</dev/null; [ -n "$v" ] && echo set`, "set\n"},
		{"a function keeps the name", `f() { :; }; f {v}</dev/null; [ -n "$v" ] && echo set`, "set\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := runFdVar(t, t.TempDir(), func(s *Semantics) {
				s.FdVariableOutlivesTheCommand = Yes
			}, c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// The spellings only some grammars take: an element and a positional
// parameter are put back as well as a plain name is. Each row ends with the
// builtin control, which must move the same target — without it, a grammar
// that did not take the spelling would pass the row by never storing at all.
func TestASpelledFdVariableOnAProgramIsThatProgramsOwn(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		grammar         func(*syntax.Dialect)
	}{
		{
			"an element", `a=(p q); cat {a[1]}</dev/null </dev/null; echo "${a[*]}"
: {a[1]}</dev/null; [ "${a[1]}" != q ] && echo moved`, "p q\nmoved\n",
			func(d *syntax.Dialect) { d.FdVariableSubscript = true },
		},
		{
			"a positional parameter", `set -- a b; cat {1}</dev/null </dev/null; echo "$1"
: {1}</dev/null; [ "$1" != a ] && echo moved`, "a\nmoved\n",
			func(d *syntax.Dialect) { d.FdVariablePositional = true },
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := syntax.Core()
			c.grammar(&d)
			out := runFdVarIn(t, d, t.TempDir(), func(s *Semantics) {
				s.FdVariableOutlivesTheCommand = Yes
			}, c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// Neither a program's `{name}` nor anything else about it asks the outliving
// axis: every shell closes the descriptor with the process, so an unanswered
// axis must still let the command run.
func TestAnFdVariableOnAProgramAsksNothing(t *testing.T) {
	out := runFdVar(t, t.TempDir(), func(s *Semantics) {
		s.FdVariableOutlivesTheCommand = Unspecified
	}, `cat {v}</dev/null </dev/null; echo "st=$? [${v-unset}]"`)
	if want := "st=0 [unset]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
