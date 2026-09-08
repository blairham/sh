// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// An operator over `$@` or `$*` applies to each positional parameter, exactly
// as it does to each element of an array.
//
// It did not. The node fell through to the scalar path, where `$@` is the
// parameters *joined*, so the operator ran once against that one string and
// the expansion came back as a single field — at status 0, with a plausible
// value standing where a list should have been.
//
// Measured 2026-09-08 on `set -- ax bx cx`. The prefix trims and the
// word-substituting operators are unanimous across the whole panel --
// `printf '[%s]' "${@#a}"` is `[x][bx][cx]` in bash 5.3.15, that binary under
// argv[0] `sh`, bash 3.2.57, dash, ksh93 and zsh 5.9.2 alike, and so are
// `"${*#a}"`, `${@-d}` and `${@:-d}` -- so this is a core rule and not a
// Semantics axis. See the corpus row
// `core/an-operator-over-the-positional-parameters-in-quotes`.
//
// Two of the rows below reach past dash and say so:
//
//   - `${@//x/Y}` is a bad substitution in dash, which has no replacement
//     operator at all. The five that have one distribute it.
//   - `${@%a}` is where dash is alone and wrong rather than different: on
//     `set -- xa xb xc` it answers the single field `x`, dropping two
//     parameters outright, where the other five answer `[x][xb][xc]`. dash's
//     own `"$@"` keeps three fields, so this is not a reading dash holds
//     consistently and it is left out of the corpus rather than recorded as
//     an axis.
//
// What made it survive is that the two readings agree whenever the operator
// happens to be a no-op on the join, and that the *unquoted* spelling hides
// the difference even when it is not: a joined `x bx cx` is cut back into the
// same three words by field splitting. So the quoted spelling and the prefix
// trim are the pair that tells the readings apart, and a probe built on
// `${@%x}` unquoted confirms a shell that has the rule backwards.
func TestAnOperatorDistributesOverThePositionalParameters(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The trims, each in both lengths.
		{`set -- ax bx cx; printf '[%s]' ${@#a}`, "[x][bx][cx]"},
		{`set -- ax bx cx; printf '[%s]' ${@##a}`, "[x][bx][cx]"},
		{`set -- xa xb xc; printf '[%s]' ${@%a}`, "[x][xb][xc]"},
		{`set -- xa xb xc; printf '[%s]' ${@%%a}`, "[x][xb][xc]"},
		// The replacements.
		{`set -- ax bx cx; printf '[%s]' ${@//x/Y}`, "[aY][bY][cY]"},
		{`set -- ax bx cx; printf '[%s]' ${@/x/Y}`, "[aY][bY][cY]"},
		// And the word-substituting operators, which yield the parameters
		// themselves when there are any.
		{`set -- ax bx cx; printf '[%s]' ${@:-d}`, "[ax][bx][cx]"},
		{`set -- ax bx cx; printf '[%s]' ${@-d}`, "[ax][bx][cx]"},
		// `$*` takes every one of them the same way.
		{`set -- ax bx cx; printf '[%s]' ${*#a}`, "[x][bx][cx]"},
		{`set -- ax bx cx; printf '[%s]' ${*//x/Y}`, "[aY][bY][cY]"},
	} {
		out, st := run(t, c.src, nil)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
		if st != 0 {
			t.Errorf("%s: status = %d, want 0", c.src, st)
		}
	}
}

// Quoted, `"$@"` keeps one field per parameter and `"$*"` joins them — the
// same division the two spellings already keep without an operator, and the
// reason the rewrite gives each name its own subscript rather than one.
//
// Measured with the same panel: `printf '[%s]' "${@#a}"` is `[x][bx][cx]` and
// `"${*#a}"` is the single field `[x bx cx]`. A rewrite that sent both to
// `[@]` answers the first row right and the second with three fields.
func TestQuotingTellsTheTwoSpellingsApartUnderAnOperator(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`set -- ax bx cx; printf '[%s]' "${@#a}"`, "[x][bx][cx]"},
		{`set -- ax bx cx; printf '[%s]' "${*#a}"`, "[x bx cx]"},
		{`set -- ax bx cx; printf '[%s]' "${@:-d}"`, "[ax][bx][cx]"},
	} {
		out, _ := run(t, c.src, nil)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
	}
}

// With no parameters at all the expansion is no field, not one empty one —
// which is the shape `set -- "${@#a}"` depends on and the one the scalar path
// cannot say.
//
// Counted through `set --` rather than printed, because `printf` runs its
// format once whatever it is given: `printf '[%s]' "${@#a}"` is `[]` on an
// empty list in every shell in the panel and in a shell that answered with
// one empty field, so it cannot tell the two apart.
func TestAnOperatorOverNoPositionalParametersIsNoField(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`set --; set -- "${@#a}"; echo $#`, "0"},
		{`set --; set -- ${@#a}; echo $#`, "0"},
		// And the word still fires, because there is nothing to substitute.
		{`set --; set -- ${@:-d}; echo "$# $1"`, "1 d"},
	} {
		out, _ := run(t, c.src, nil)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
	}
}

// The three element-selecting operators choose among the parameters, which is
// the shape a plugin manager's option parser is built on: `set -- ${@:#--}`
// after `zparseopts` is how `~/.zi/bin/zi.zsh` drops the separator from the
// names it was given. Joining first left the whole list as one word — so a
// declaration of several autoloadable functions became a single function
// whose name held all of them, at status 0.
//
// zsh's grammar alone has the operators, so the rows are enabled by name.
func TestTheElementSelectorsChooseAmongThePositionalParameters(t *testing.T) {
	sel := func(t *testing.T, src string) (string, int) {
		t.Helper()
		return runGrammar(t, src, func(d *syntax.Dialect) {
			d.ParamElementSelection = true
		}, nil)
	}
	for _, c := range []struct{ src, want string }{
		{`set -- a b c; printf '[%s]' ${@:#b}`, "[a][c]"},
		{`set -- a -- b; printf '[%s]' ${@:#--}`, "[a][b]"},
		{`x=(b); set -- a b c; printf '[%s]' ${@:|x}`, "[a][c]"},
		{`x=(b); set -- a b c; printf '[%s]' ${@:*x}`, "[b]"},
	} {
		out, st := sel(t, c.src)
		if got := strings.TrimSpace(out); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
		if st != 0 {
			t.Errorf("%s: status = %d, want 0", c.src, st)
		}
	}
}

// A dialect that reads a bare array name as one value still distributes over
// the *parameters*, because the two are not the same question: the panel
// splits on the array name and is unanimous on `$@`.
//
// The guard this pins is the one a shared rewrite would lose. `$@` must not
// be routed through bareArrayAsList's axis — measured, dash has no arrays to
// answer that axis with and distributes over `$@` regardless.
func TestThePositionalsDistributeWithoutAskingTheArrayNameAxis(t *testing.T) {
	out, _ := run(t, `a=x; set -- ax bx; printf '[%s]' ${@#a}; printf '[%s]' ${a}`, nil)
	if got := strings.TrimSpace(out); got != "[x][bx][x]" {
		t.Errorf("got %q, want the parameters distributed and the scalar left alone", got)
	}
}

// `${#@}` is still the *count* of parameters and not the width of anything,
// which is the one shape the rewrite must leave alone: it is answered before
// an operator is read, and giving it a subscript would send it to the array
// path.
func TestTheLengthOfThePositionalsIsStillTheirCount(t *testing.T) {
	out, _ := run(t, `set -- ax bx cx; echo "${#@} ${#*} ${#1}"`, nil)
	if got := strings.TrimSpace(out); got != "3 3 2" {
		t.Errorf("got %q, want the two counts and one width", got)
	}
}
