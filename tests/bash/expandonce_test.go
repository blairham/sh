// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// `shopt -s assoc_expand_once` — and `array_expand_once`, which is the same
// switch under a second name — withholds the second round a subscript gets
// when it reaches a builtin as text.
//
// Measured 2026-09-19 on bash 5.3.20, each probe from a script file with
// standard input on /dev/null, over `declare -A m; k='x y'; m[$k]=hello`.
// The left column is the default and the right the same probe under
// `-O assoc_expand_once`:
//
//	unset -v 'm[$k]'      the element is gone   it survives
//	unset 'm[$k]'         the element is gone   it survives
//	printf -v 'm[$k]' P   the key `x y`         the key `$k`
//	read 'm[$k]' <<<Z     the key `x y`         the key `$k`
//	test -v 'm[$k]'       true                  false
//	[ -v 'm[$k]' ]        true                  false
//
// See interp.Runner.ExpandsAnOperandsSubscriptAgain, and #3298, which this
// closes.
func TestTheExpandOnceOptionWithholdsABuiltinsSecondRound(t *testing.T) {
	const table = `declare -A m; k='x y'; m[$k]=hello; `
	const keys = `; for q in "${!m[@]}"; do printf '[%s]' "$q"; done`
	for _, c := range []struct{ name, src, off, on string }{
		{
			"unset with the letter",
			table + `unset -v 'm[$k]'; printf '[%s]' "${m[$k]-GONE}"`,
			"[GONE]", "[hello]",
		},
		{
			"unset without it",
			table + `unset 'm[$k]'; printf '[%s]' "${m[$k]-GONE}"`,
			"[GONE]", "[hello]",
		},
		{
			"printf through a name operand",
			`declare -A m; k='x y'; printf -v 'm[$k]' P` + keys,
			"[x y]", "[$k]",
		},
		{
			"read through a name operand",
			`declare -A m; k='x y'; read 'm[$k]' <<<Z` + keys,
			"[x y]", "[$k]",
		},
		{
			"the is-set builtin",
			table + `test -v 'm[$k]' && echo SET || echo UNSET`,
			"SET\n", "UNSET\n",
		},
		{
			"the is-set builtin spelled with brackets",
			table + `[ -v 'm[$k]' ] && echo SET || echo UNSET`,
			"SET\n", "UNSET\n",
		},
		// The control every row above needs: an operand with nothing in it
		// for a round to do reaches the same element under either state.
		// Without it a shell that had stopped reading a quoted subscript at
		// all would pass the right-hand column everywhere.
		{
			"an operand with nothing to expand",
			`declare -A m; m[plain]=hello; unset 'm[plain]'; printf '[%s]' "${m[plain]-GONE}"`,
			"[GONE]", "[GONE]",
		},
		{
			"a store with nothing to expand",
			`declare -A m; read 'm[plain]' <<<Z; printf '[%s]' "${m[plain]-U}"`,
			"[Z]", "[Z]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := answersRun(t, c.src); out != c.off || st != 0 {
				t.Errorf("default: %s = %q status %d, want %q at 0", c.src, out, st, c.off)
			}
			withOption := `shopt -s assoc_expand_once; ` + c.src
			if out, st := answersRun(t, withOption); out != c.on || st != 0 {
				t.Errorf("assoc_expand_once: %s = %q status %d, want %q at 0", c.src, out, st, c.on)
			}
			// And the synonym is the same switch and not a second one.
			withSynonym := `shopt -s array_expand_once; ` + c.src
			if out, st := answersRun(t, withSynonym); out != c.on || st != 0 {
				t.Errorf("array_expand_once: %s = %q status %d, want %q at 0", c.src, out, st, c.on)
			}
		})
	}
}

// The two neighboring surfaces keep rounding with the option set, which is
// what says the switch is scoped to the four the option was measured to move.
//
// Measured 2026-09-19 on bash 5.3.20 with `-O assoc_expand_once`: `declare
// 'd[$k]'=Q` still writes the key `x y` and `[[ -v 'm[$k]' ]]` still finds
// it. A switch wired at the shared round rather than at the four sites would
// move both, and every other row in this file would still pass.
func TestTheOptionLeavesTheNeighbouringSurfacesRounding(t *testing.T) {
	const keys = `; for q in "${!d[@]}"; do printf '[%s]' "$q"; done`
	for _, c := range []struct{ name, src, want string }{
		{
			"a declaration's operand",
			`declare -A d; k='x y'; declare 'd[$k]'=Q` + keys,
			"[x y]",
		},
		{
			"the is-set keyword",
			`declare -A m; k='x y'; m[$k]=V; [[ -v 'm[$k]' ]] && printf SET || printf UNSET`,
			"SET",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, prefix := range []string{"", "shopt -s assoc_expand_once; ", "shopt -s array_expand_once; "} {
				if out, st := answersRun(t, prefix+c.src); out != c.want || st != 0 {
					t.Errorf("%q%s = %q status %d, want %q at 0", prefix, c.src, out, st, c.want)
				}
			}
		})
	}
}

// The two names are one switch, and the listing says so.
//
// Measured 2026-09-19 on bash 5.3.20, from `-c`: setting either name reports
// both `on`, unsetting either reports both `off`, and `$BASHOPTS` carries the
// name only while it is set. The refusal this replaces was `shopt:
// assoc_expand_once: not implemented` at 1 with the listing reading `off`,
// for a state the shell was already in (#3298).
func TestTheExpandOnceNamesAreOneSwitch(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"setting one reports 0", `shopt -s assoc_expand_once; echo st=$?`, "st=0\n"},
		{
			"setting one sets the other",
			`shopt -s assoc_expand_once; shopt array_expand_once`,
			"array_expand_once   \ton\n",
		},
		{
			"setting the other sets the one",
			`shopt -s array_expand_once; shopt assoc_expand_once`,
			"assoc_expand_once   \ton\n",
		},
		{
			"unsetting one unsets the other",
			`shopt -s array_expand_once; shopt -u assoc_expand_once; shopt array_expand_once`,
			"array_expand_once   \toff\n",
		},
		{"the default is off", `shopt assoc_expand_once`, "assoc_expand_once   \toff\n"},
		{"the printable form", `shopt -s assoc_expand_once; shopt -p assoc_expand_once`, "shopt -s assoc_expand_once\n"},
		{"the printable form when off", `shopt -p array_expand_once`, "shopt -u array_expand_once\n"},
		{
			"the environment listing carries it",
			`shopt -s array_expand_once; case ":$BASHOPTS:" in *:assoc_expand_once:*) echo IN ;; *) echo OUT ;; esac`,
			"IN\n",
		},
		{
			"and not while it is off",
			`case ":$BASHOPTS:" in *:assoc_expand_once:*) echo IN ;; *) echo OUT ;; esac`,
			"OUT\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := answersRun(t, c.src)
			// `shopt -p` over an unset name reports 1, which is the status
			// the listing carries rather than a failure.
			if out != c.want {
				t.Errorf("%s = %q status %d, want %q", c.src, out, st, c.want)
			}
		})
	}
}

// The three axes this dialect answers for the builtins' round, pinned where
// the rest of them are.
func TestThisDialectRoundsABuiltinsSubscriptText(t *testing.T) {
	s := bash.Semantics()
	for _, c := range []struct {
		name string
		got  interp.Answer
	}{
		{"UnsetExpandsAFlatSubscript", s.UnsetExpandsAFlatSubscript},
		{"OutputOperandExpandsAFlatSubscript", s.OutputOperandExpandsAFlatSubscript},
		{"TestIsSetExpandsAFlatSubscript", s.TestIsSetExpandsAFlatSubscript},
	} {
		if c.got != interp.Yes {
			t.Errorf("%s = %v, want %v", c.name, c.got, interp.Yes)
		}
	}
}
