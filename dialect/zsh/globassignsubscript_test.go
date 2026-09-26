// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `GLOB_ASSIGN` reaches a **subscripted** assignment too, and there it is a
// *splice* rather than a store: the match takes the element's place and pushes
// the rest along, so one subscript goes in and three elements come out (#4658).
//
// Measured 2026-09-26 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — run `-f` over a script file, in the directory
// globTree builds.
//
// **Both states, every row**, because the option is what reaches this and a
// grid run only with it on cannot tell a splice from a shell that ignores the
// option and happens to agree.
func TestGlobAssignSplicesASubscriptedAssignment(t *testing.T) {
	for _, c := range []struct{ name, src, off, on string }{
		{
			// The row that names the shape: one subscript in, three
			// elements out.
			"several matches splice",
			"a=(q w e)\na[2]=*.txt",
			"typeset -a a=( q '*.txt' e )\n",
			"typeset -a a=( q a.txt b.txt c.txt e )\n",
		},
		{
			// One match is the road where the splice and a plain store
			// coincide.
			"one match is a plain store",
			"a=(q w e)\na[2]=one.*",
			"typeset -a a=( q 'one.*' e )\n",
			"typeset -a a=( q one.only e )\n",
		},
		{
			// **The control that says it is about the match**: a value with
			// no metacharacter in it is the same store in both states.
			"a plain value does not move",
			"a=(q w e)\na[2]=plain",
			"typeset -a a=( q plain e )\n",
			"typeset -a a=( q plain e )\n",
		},
		{
			// An append puts the match *after* the element rather than
			// joining it, which is the element-literal rule reached by the
			// other spelling.
			"an append places the match after the element",
			"a=(q w e)\na[2]+=*.txt",
			"typeset -a a=( q 'w*.txt' e )\n",
			"typeset -a a=( q w a.txt b.txt c.txt e )\n",
		},
		{
			// A range names a span, and the match replaces the whole span.
			"a range replaces the span",
			"a=(q w e)\na[1,2]=*.txt",
			"typeset -a a=( '*.txt' e )\n",
			"typeset -a a=( a.txt b.txt c.txt e )\n",
		},
		{
			// Past the last element the gap is padded first, exactly as the
			// literal spelling pads it.
			"a subscript past the end pads and then places",
			"a=(q w e)\na[5]=*.txt",
			"typeset -a a=( q w e '' '*.txt' )\n",
			"typeset -a a=( q w e '' a.txt b.txt c.txt )\n",
		},
		{
			// A quoted pattern is not a pattern, in either state — the same
			// control the statement form has.
			"a quoted value keeps its characters",
			"a=(q w e)\na[2]=\"*.txt\"",
			"typeset -a a=( q '*.txt' e )\n",
			"typeset -a a=( q '*.txt' e )\n",
		},
		{
			// Nor is one a value merely carried, which is what says the
			// marks and not the characters decide.
			"a value carrying the characters is not a pattern",
			"a=(q w e)\np=\"*.txt\"\na[2]=$p",
			"typeset -a a=( q '*.txt' e )\n",
			"typeset -a a=( q '*.txt' e )\n",
		},
		{
			// `unsetopt glob` reaches it, as it reaches the statement form.
			"the glob option still switches it off",
			"unsetopt glob\na=(q w e)\na[2]=*.txt",
			"typeset -a a=( q '*.txt' e )\n",
			"typeset -a a=( q '*.txt' e )\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, state := range []struct{ name, setopt, want string }{
				{"off", "unsetopt globassign\n", c.off},
				{"on", "setopt globassign\n", c.on},
			} {
				t.Run(state.name, func(t *testing.T) {
					root := globTree(t)
					out, _ := runZsh(t, root, state.setopt+c.src+"\ntypeset -p a\n")
					if out != state.want {
						t.Errorf("got %q, want %q", out, state.want)
					}
				})
			}
		})
	}
}

// A target with no room for a list is refused by name rather than quietly
// storing the pattern's text, which is a plausible wrong answer at status 0.
//
// A table's key holds one value and a string's position holds one character,
// so the reference refuses — and the two sentences are the ones the literal
// spelling of the same shape already earns.
//
// **The single-match rows are the controls**, and they are what keep the
// refusal keyed on the *list* rather than on the option or on the pattern:
// one match is one value and every target can hold it.
func TestGlobAssignRefusesATargetThatCannotHoldTheMatch(t *testing.T) {
	for _, c := range []struct {
		name, src, refusal, want string
	}{
		{
			"a table key cannot hold several",
			"typeset -A h\nh=(k v)\nh[k]=*.txt\ntypeset -p h",
			"h: attempt to set slice of associative array", "",
		},
		{
			"a table key holds one",
			"typeset -A h\nh=(k v)\nh[k]=one.*\ntypeset -p h",
			"", "typeset -A h=( [k]=one.only )\n",
		},
		{
			"a string position cannot hold several",
			"v=abcdef\nv[2]=*.txt\ntypeset -p v",
			"v: attempt to assign array value to non-array", "",
		},
		{
			"a string position holds one",
			"v=abcdef\nv[2]=one.*\ntypeset -p v",
			"", "typeset v=aone.onlycdef\n",
		},
		{
			// An unset name is neither: it becomes an array, with the
			// padding in front of the subscript.
			"an unset name becomes an array",
			"n[2]=*.txt\ntypeset -p n",
			"", "typeset -a n=( '' a.txt b.txt c.txt )\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := globTree(t)
			out, st, errs := runZshSplit(t, root, "setopt globassign\n"+c.src+"\n")
			if c.refusal != "" {
				if !strings.Contains(errs, c.refusal) {
					t.Errorf("stderr = %q, want %q in it", errs, c.refusal)
				}
				if out != "" {
					t.Errorf("stdout = %q, want nothing — the shell leaves", out)
				}
				if st != 1 {
					t.Errorf("status = %d, want 1", st)
				}
				return
			}
			if out != c.want {
				t.Errorf("stdout = %q, want %q (stderr %q)", out, c.want, errs)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// A miss is the same refusal any other unmatched pattern earns, and it reaches
// a subscripted store through the same road as the statement form — which is
// what says the pattern really is being globbed here rather than being read as
// text and matched afterwards.
func TestGlobAssignRefusesAMissInASubscriptedAssignment(t *testing.T) {
	root := globTree(t)
	out, st, errs := runZshSplit(t, root,
		"setopt globassign\na=(q w e)\na[2]=*.nomatch\nprint reached\n")
	if !strings.Contains(errs, "no matches found") {
		t.Errorf("stderr = %q, want the miss", errs)
	}
	if out != "" {
		t.Errorf("stdout = %q, want nothing — the shell leaves", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
	// The control: `nullglob` empties the word instead, so the row above is
	// the default reading rather than the only one.
	out, st, errs = runZshSplit(t, globTree(t),
		"setopt globassign\nsetopt nullglob\na=(q w e)\na[2]=*.nomatch\ntypeset -p a\n")
	if want := "typeset -a a=( q '' e )\n"; out != want {
		t.Errorf("stdout = %q, want %q (stderr %q)", out, want, errs)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The subscript is expanded **once**, which a splice reached by a second read
// of the same word would break silently: `a[$(f)]=*.txt` would run `f` twice
// and still place the match in the right position.
func TestASplicedSubscriptIsExpandedOnce(t *testing.T) {
	root := globTree(t)
	out, _, errs := runZshSplit(t, root,
		"setopt globassign\nf() { print -u2 RAN; print 2; }\na=(q w e)\na[$(f)]=*.txt\ntypeset -p a\n")
	if want := "typeset -a a=( q a.txt b.txt c.txt e )\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if got := strings.Count(errs, "RAN"); got != 1 {
		t.Errorf("the subscript ran its command %d times, want 1: %q", got, errs)
	}
}
