// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// The appending spelling of a reference declaration **joins** the text onto
// what the reference is already pointing at. It is not a second way to aim.
//
// `typeset -n ref=var; typeset -n ref+=[@]` leaves the reference aimed at
// `var[@]`, so a read through it reads the whole array. This shell re-aimed at
// the fragment — the standing target was thrown away and `[@]` alone was then
// weighed as a name, which it is not, so the line was refused as well as
// wrong.
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C bash f.sh`
// over script files, against bash 5.3.20:
//
//	typeset -n ref=var; typeset -n ref+=[@]     declare -n ref="var[@]"
//	typeset -n ref=v; typeset -n ref+=a
//	  typeset -n ref+=b                         declare -n ref="vab"
//	va=HI; typeset -n ref=v; typeset -n ref+=a  $ref reads HI
//	plain=tgt; typeset -n plain+=2              declare -n plain="tgt2"
//
// ksh93u+ 2012-08-01 has no appending spelling on a declaration at all — every
// one of those lines is `typeset: ref+: is not an identifier` there, which
// this shell's ksh column already answers — so there is no axis here (#4178).
func TestAnAppendingReferenceDeclarationJoinsOntoItsTarget(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The row `nameref15.sub` grades: a subscript joined onto a
			// plain name.
			"a subscript joins onto the name",
			"var=abc\ntypeset -n ref=var\ntypeset -n ref+='[@]'\ndeclare -p ref",
			`declare -n ref="var[@]"`,
		},
		{
			// And the join is what a read then goes through, which is the
			// half a listing alone would not catch.
			"and the join is what a read follows",
			"declare -a v=(p q r)\ntypeset -n ref=v\ntypeset -n ref+='[1]'\nprintf '[%s]' \"$ref\"",
			`[q]`,
		},
		{
			// Twice, so the base is the standing target rather than the
			// name's original value.
			"joining twice builds the name up",
			"typeset -n ref=v\ntypeset -n ref+=a\ntypeset -n ref+=b\ndeclare -p ref",
			`declare -n ref="vab"`,
		},
		{
			// A name that is **not** a reference joins onto the value it
			// holds, which is the same cell the valueless form adopts.
			"a plain name joins onto its value",
			"plain=tgt\ntypeset -n plain+=2\ndeclare -p plain",
			`declare -n plain="tgt2"`,
		},
		{
			// Nothing to join onto leaves the fragment standing alone, so
			// the ordinary spelling is the empty base of this one.
			"a name with nothing held joins onto nothing",
			"typeset -n ref+=var\ndeclare -p ref",
			`declare -n ref="var"`,
		},
		{
			// An empty fragment is a join that changes nothing rather than a
			// bad target — which this shell refused, because an empty word is
			// exactly what the aiming spelling will not take.
			"an empty fragment leaves the target alone",
			"typeset -n ref=var\ntypeset -n ref+=\nprintf '[%d]' $?\ndeclare -p ref",
			`[0]declare -n ref="var"`,
		},
		{
			// The binding `local` makes has never held anything, so the
			// caller's value of the same spelling is not the base. This is
			// the `adopts` gate the valueless form already keeps.
			"a fresh local joins onto nothing, not onto the caller",
			"ref=abc\nf() { local -n ref+=X; declare -p ref; }\nf",
			`declare -n ref="X"`,
		},
		{
			// What the child is handed is the joined target, since the
			// exported entry is the text the reference is aimed at.
			"an exported reference carries the join",
			"typeset -nx ref=v\ntypeset -nx ref+=a\nenv | grep '^ref='",
			"ref=va",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want it to contain %q", out, tc.want)
			}
		})
	}
}

// What the join is weighed against, and in which order.
//
// The joined word goes through the same checks a written one does — so a join
// that is not a name is refused and a join that reaches the name itself is a
// self reference — but two of those checks move in front of it, because this
// spelling has no word at all until the name's own contents have been read.
//
// Measured 2026-09-23 against bash 5.3.20:
//
//	v=1; declare -rn r=v; declare -n r=1x    `1x': invalid variable name…
//	v=1; declare -rn r=v; declare -n r+=x    r: readonly variable
//	declare -a a=(x); typeset -n a=1b        `1b': invalid variable name…
//	declare -a a=(x); typeset -n a+=b        a: reference variable cannot be
//	                                         an array
//
// So the written spelling weighs the word it was handed and never looks at the
// name; the appending spelling settles the name — its array-ness, then its
// freeze — and only then is there a word.
//
// The refusal's **sentence** is deliberately left where it stands: bash draws
// the declaration's ordinary `not a valid identifier` for a fragment and the
// `n` letter's own sentence for a whole word, and which of the two a refusal
// takes belongs with the rest of the nameref wordings rather than with this
// rule. What is pinned here is that the word it *names* is the fragment the
// operand carried and not the join.
func TestAnAppendingReferenceDeclarationWeighsTheJoin(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// A join that is not a name is refused and the standing target
			// is kept, which is the take-back every reported refusal makes.
			"a join that is not a name is refused",
			"var=abc\ntypeset -n ref=var\ntypeset -n ref+=' bad'\nprintf '[%d]' $?\ndeclare -p ref",
			`[1]declare -n ref="var"`,
		},
		{
			// And the word it names is the fragment, not the join.
			"the refusal names the fragment",
			"typeset -n ref=v\ntypeset -n ref+='-x'\ndeclare -p ref",
			"`-x'",
		},
		{
			// A join that reaches the name itself is the ordinary self
			// reference, which at the top level is refused.
			"a join that reaches the name is a self reference",
			"typeset -n ref=r\ntypeset -n ref+=ef\nprintf '[%d]' $?",
			"nameref variable self references not allowed",
		},
		{
			// And that refusal is spoken as the **shell**: this spelling
			// writes no builtin's name in front of it, where the written
			// spelling does.
			"the self reference is spoken by the shell",
			"typeset -n ref=r\ntypeset -n ref+=ef",
			": ref: nameref variable self references not allowed",
		},
		{
			// One warning inside a function, where the written spelling
			// writes two. Same fact as the row above: the builtin's own copy
			// is the one that is not written.
			"and warns once inside a function",
			"f() { local -n ref=r; local -n ref+=ef; }\nf 2>&1 | grep -c 'circular name reference'",
			"1",
		},
		{
			// The freeze is settled before the join too, which the
			// attribute-over-a-frozen-name refusal one level up already
			// does for this spelling — pinned here because it is the
			// observable order, not because this change enforces it.
			"a frozen reference refuses before the join is weighed",
			"v=1\ndeclare -rn r=v\ndeclare -n r+=' x'\nprintf '[%d]' $?",
			"r: readonly variable",
		},
		{
			// And the array-ness before that. The fragment has to be one
			// the join would refuse, or the row cannot tell the orders
			// apart: with a good join the late reading answers the same
			// sentence anyway, and a mutation that removed the early
			// refusal went unnoticed.
			"an array refuses before the join is weighed",
			"declare -a a=(x)\ntypeset -n a+='-b'\nprintf '[%d]' $?",
			"a: reference variable cannot be an array",
		},
		{
			// The good-join half of that pair, which agrees either way and
			// is here as the control rather than as the pin.
			"and refuses a join that would have been a good target",
			"declare -a a=(x)\ntypeset -n a+=b\nprintf '[%d]' $?",
			"a: reference variable cannot be an array",
		},
		{
			// The control for the two rows above: the **written** spelling
			// over the same array takes the bad target instead, so the order
			// really is this spelling's own.
			"the written spelling still weighs its word first",
			"declare -a a=(x)\ntypeset -n a=1b",
			"`1b': invalid variable name for name reference",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("= %q, want it to contain %q", out, tc.want)
			}
		})
	}
}
