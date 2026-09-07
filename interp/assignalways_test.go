// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// alwaysAssigning is the grammar these expansions need, named by the
// constructs rather than by a shell.
func alwaysAssigning(d *syntax.Dialect) {
	d.ParamAssignAlways = true
	d.ArrayLiteral = true
	d.ArraySubscript = true
	d.ParamExpansionFlags = true
}

// `${name::=word}` assigns every time and substitutes what it assigned.
//
// The operator that asks nothing. Every row is a measurement on zsh 5.9.2,
// the only shell in the panel with the construct — the other four read the
// same characters as a substring and fail in arithmetic there, which is what
// this shell did for eighteen lines of one real startup (#1369).
//
// The three states of the parameter are the point: `${v:=w}` assigns on two
// of them and `${v=w}` on one, and this operator assigns on all three. A
// reading that answered from the value on the third would be right twice and
// plausible the third time.
func TestTheAlwaysAssignOperatorAssignsEveryTime(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"unset", `unset v; printf "[%s][%s]" "${v::=new}" "$v"`, "[new][new]"},
		{"set and empty", `v=; printf "[%s][%s]" "${v::=new}" "$v"`, "[new][new]"},
		{"set and not empty", `v=old; printf "[%s][%s]" "${v::=new}" "$v"`, "[new][new]"},
		// The row that separates it from `:=`, which is the operator it is
		// most easily read as: the same line with one colon keeps `old`.
		{"where the colon assignment would not", `v=old; printf "[%s][%s]" "${v:=new}" "$v"`, "[old][old]"},
		{"an empty word assigns emptiness", `v=old; printf "[%s][%s]" "${v::=}" "$v"`, "[][]"},
		{"the word is expanded", `w=W; v=old; printf "[%s][%s]" "${v::=$w-x}" "$v"`, "[W-x][W-x]"},
		{"and a command substitution in it runs", `v=old; printf "[%s][%s]" "${v::=$(echo sub)}" "$v"`, "[sub][sub]"},
		// Twice on one line, which is what says the assignment is not
		// cached: the second reads what the first left.
		{"twice on one line", `printf "[%s]" ${v::=a}${v::=b}$v`, "[abb]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, alwaysAssigning, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A subscript makes the assignment the *element's*, exactly as it is for the
// conditional assignment beside it. Measured on zsh 5.9.2, associative and
// indexed alike, including the two shapes that grow the array.
func TestTheAlwaysAssignOperatorReachesAnElement(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an existing association key",
			`typeset -A Z; Z[k]=old; printf "[%s][%s]" "${Z[k]::=new}" "${Z[k]}"`,
			"[new][new]",
		},
		{
			"a key that was not there",
			`typeset -A Z; printf "[%s][%s]" "${Z[new]::=v}" "${Z[new]}"`,
			"[v][v]",
		},
		{
			// The key the plugin manager's formatter writes: a hyphenated
			// name, which is a key in an association and arithmetic in an
			// indexed array.
			"a hyphenated key",
			`typeset -A Z; printf "[%s][%s]" "${Z[last-code]::=rst}" "${Z[last-code]}"`,
			"[rst][rst]",
		},
		{
			// The subscript is read by whatever base the semantics carry, so
			// this asserts the element the *same* subscript reads back rather
			// than a position — the base is a dialect's answer and not this
			// operator's business.
			"an indexed element",
			`a=(1 2 3); printf "[%s][%s]" "${a[2]::=new}" "${a[2]}"`,
			"[new][new]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, alwaysAssigning, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The parameter has to be one an assignment can name, and a refusal is what
// says so — not a substitution of the word at status 0, which is the shape a
// later read of the parameter would find nothing behind.
//
// Measured on zsh 5.9.2: `${@::=w}`, `${*::=w}`, `${#::=w}`, `${?::=w}` and
// `${-::=w}` are each `not an identifier: <name>` and each ends the script at
// status 1, while a name, a positional and `0` all assign.
func TestTheAlwaysAssignOperatorRefusesAParameterItCannotName(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the parameter list", `echo "${@::=new}"`, "not an identifier: @"},
		{"the joined parameter list", `echo "${*::=new}"`, "not an identifier: *"},
		{"the count", `echo "${#::=new}"`, "not an identifier: #"},
		{"the last status", `echo "${?::=new}"`, "not an identifier: ?"},
		{"the option letters", `echo "${-::=new}"`, "not an identifier: -"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src+`; echo AFTER`, alwaysAssigning, nil)
			if !strings.Contains(out, tc.want) {
				t.Errorf("output = %q, want %q in it", out, tc.want)
			}
			if strings.Contains(out, "AFTER") {
				t.Errorf("output = %q, want the script ended there", out)
			}
			if st == 0 {
				t.Errorf("status 0, want the refusal to be fatal")
			}
		})
	}
}

// A positional and `0` are names an assignment may reach, so they are *not*
// refused. Measured on zsh 5.9.2: `set -- P Q; ${1::=new}` and `${0::=new}`
// each substitute `new` at status 0 where `${#::=new}` refuses.
//
// Only the substitution and the absence of a refusal are asserted here. Where
// the value *lands* is a second question this engine does not yet answer the
// same way — an assignment through an expansion does not reach the positional
// list, which `${1:=new}` has always got wrong too and which
// docs/spec/grammar/parameter-expansion.md records under "What this
// implementation does not match" (#1389). Asserting the landing would pin the
// divergence as though it were the answer; asserting the refusal's absence is
// the part that is measured and settled.
func TestAPositionalIsNotRefused(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"a positional", `set -- P Q; printf "[%s]" "${1::=new}"`},
		{"one past the end", `set -- P Q; printf "[%s]" "${9::=new}"`},
		{"and zero", `printf "[%s]" "${0::=new}"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, alwaysAssigning, nil)
			if out != "[new]" || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, "[new]")
			}
		})
	}
}

// The word is expanded before the parameter is checked, so a command
// substitution in it runs even on the line that then fails. Measured on
// zsh 5.9.2: `${#::=$(echo RAN >&2)}` writes RAN and then refuses the name.
//
// Asserted because the tidier code is the other order, and the other order is
// a side effect this shell would silently skip.
func TestTheWordRunsBeforeTheNameIsChecked(t *testing.T) {
	out, st := runGrammar(t, `echo "${#::=$(echo RAN >&2)}"`, alwaysAssigning, nil)
	if !strings.Contains(out, "RAN") {
		t.Errorf("output = %q, want the word's own output in it", out)
	}
	if !strings.Contains(out, "not an identifier: #") {
		t.Errorf("output = %q, want the refusal in it", out)
	}
	if st == 0 {
		t.Errorf("status 0, want the refusal to be fatal")
	}
}

// `set -u` has nothing to be about here: the operator never reads the value.
// Measured — `setopt nounset; unset v; ${v::=new}` is `new` at status 0.
func TestTheAlwaysAssignOperatorIsNotAnUnsetRead(t *testing.T) {
	out, st := runGrammar(t, `set -u; unset v; printf "[%s][%s]" "${v::=new}" "$v"`,
		alwaysAssigning, nil)
	if out != "[new][new]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[new][new]")
	}
}

// A readonly parameter refuses, through the same door every other assignment
// to one goes through. Measured fatal on zsh 5.9.2.
func TestTheAlwaysAssignOperatorRefusesAReadonly(t *testing.T) {
	out, st := runGrammar(t, `readonly v=old; echo "${v::=new}"; echo AFTER`,
		alwaysAssigning, nil)
	if strings.Contains(out, "AFTER") || st == 0 {
		t.Errorf("got %q (status %d), want the readonly refusal to be fatal", out, st)
	}
}

// A flag group in front of it still applies to what is substituted and not to
// what is stored, which is the rule the conditional assignment beside it
// already follows. Measured: `${(U)v::=abc}` is `ABC` and leaves `abc`.
func TestAFlagGroupDoesNotReachWhatIsStored(t *testing.T) {
	out, st := runGrammar(t, `printf "[%s][%s]" "${(U)v::=abc}" "$v"`, alwaysAssigning, nil)
	if out != "[ABC][abc]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[ABC][abc]")
	}
}

// And the name check holds on the flagged path too, which is a second site
// and therefore a second chance to lose it.
func TestAFlagGroupStillRefusesAParameterItCannotName(t *testing.T) {
	out, st := runGrammar(t, `echo "${(U)#::=new}"; echo AFTER`, alwaysAssigning, nil)
	if !strings.Contains(out, "not an identifier: #") {
		t.Errorf("output = %q, want the refusal in it", out)
	}
	if strings.Contains(out, "AFTER") || st == 0 {
		t.Errorf("got %q (status %d), want the refusal to be fatal", out, st)
	}
}

// The `(A)` flag is refused for this operator too, and the refusal is the
// whole point of carrying the letter at all.
//
// `(A)` makes an assignment an *array* assignment — measured on zsh 5.9.2,
// `unset u; ${(A)u::=x y}` leaves `typeset -a u=( 'x y' )`, one element, where
// the same line without the flag leaves the scalar `x y`. This engine does not
// build that, so it says so: a letter carried without its side effect would
// leave a scalar where the script asked for an array, which the first
// `${u[2]}` reads as empty and nothing before it does.
//
// Asserted for this operator and not only for `=` and `:=`, because the
// refusal asks about the *operator* and a reading that named only the
// conditional assignment would let this one through silently — which is what
// mutation says: narrowing that question to `ParamAssign` breaks no other
// test.
func TestTheArrayFlagIsRefusedOnTheAlwaysAssignToo(t *testing.T) {
	out, st := runGrammar(t, `printf "[%s]" "${(A)x::=a b}"; echo AFTER`, alwaysAssigning, nil)
	want := "the (A) expansion flag is not implemented for an assignment"
	if !strings.Contains(out, want) {
		t.Errorf("output = %q, want %q in it", out, want)
	}
	if strings.Contains(out, "AFTER") || st == 0 {
		t.Errorf("got %q (status %d), want the refusal to be fatal", out, st)
	}
}

// A length in front of the operator measures what the assignment left, which
// is the same rule every other operator under a length follows. Measured:
// `v=old; ${#v::=abcd}` is 4 and leaves `abcd`.
func TestALengthMeasuresWhatWasAssigned(t *testing.T) {
	enable := func(d *syntax.Dialect) {
		alwaysAssigning(d)
		d.ParamLengthTakesAnOperator = true
	}
	out, st := runGrammar(t, `v=old; printf "[%s][%s]" "${#v::=abcd}" "$v"`, enable, nil)
	if out != "[4][abcd]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[4][abcd]")
	}
}
