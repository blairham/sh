// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `[[ -R name ]]` asks whether a name is a **reference** — a question about
// the binding rather than about what it points at, so the reference answers
// true and its target answers false.
//
// Measured 2026-09-22 under `-c` with `LC_ALL=C` against bash 5.3.20 and
// ksh93u+ 2012-08-01, which agree on every row. #4228: the builtin spelling
// had answered since #4162 while the condition would not parse at all.
func TestANameReferenceConditionAsksAboutTheBinding(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"the reference", `v=1; declare -n r=v; [[ -R r ]] && echo REF || echo NOREF`, "REF\n"},
		{"its target", `v=1; declare -n r=v; [[ -R v ]] && echo REF || echo NOREF`, "NOREF\n"},
		{"an ordinary name", `x=1; [[ -R x ]] && echo REF || echo NOREF`, "NOREF\n"},
		{"never assigned", `[[ -R nope ]] && echo REF || echo NOREF`, "NOREF\n"},
		{"negated", `[[ ! -R nope ]] && echo NOREF || echo REF`, "NOREF\n"},
		// The operand is an ordinary word, read the way `-v`'s is.
		{"the name out of a variable", `v=1; declare -n r=v; n=r; [[ -R $n ]] && echo REF || echo NOREF`, "REF\n"},
		{"quoted is the same name", `v=1; declare -n r=v; [[ -R 'r' ]] && echo REF || echo NOREF`, "REF\n"},
		{"the empty name", `[[ -R "" ]] && echo REF || echo NOREF`, "NOREF\n"},
		// A reference whose target was never assigned is still a reference:
		// the question is about the binding, not about what it reaches.
		{"aimed at nothing", `declare -n r=nope; [[ -R r ]] && echo REF || echo NOREF`, "REF\n"},
		// A chain is a reference at every link.
		{"a chain", `v=1; declare -n r=v; declare -n s=r; [[ -R s ]] && echo REF || echo NOREF`, "REF\n"},
		// And unsetting the reference leaves no reference behind.
		{"unset again", `v=1; declare -n r=v; unset -n r; [[ -R r ]] && echo REF || echo NOREF`, "NOREF\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The `[` builtin asks the same question and has to give the same answer.
//
// This is the assertion that matters and the reason #4228 was worth its own
// issue: the two spellings reach the answer by different routes — the
// condition through the grammar's operator table, the builtin through its
// operand table — so each has its own gate, and a shell that grew one without
// the other is exactly the state this fixes. A second implementation of
// either is what would make them disagree again.
func TestTheTestBuiltinAndTheConditionAgreeAboutNameReferences(t *testing.T) {
	dir := t.TempDir()
	for _, src := range []string{
		`v=1; declare -n r=v`,
		`v=1`,
		`declare -n r=nope`,
		`v=1; declare -n r=v; unset -n r`,
		`v=1; declare -n r=v; declare -n s=r`,
	} {
		for _, name := range []string{"r", "v", "s", "nope"} {
			cond := src + `; [[ -R ` + name + ` ]] && echo REF || echo NOREF`
			builtin := src + `; [ -R ` + name + ` ] && echo REF || echo NOREF`
			a, sa := runBash(t, dir, cond)
			b, sb := runBash(t, dir, builtin)
			if a != b || sa != sb {
				t.Errorf("%s with %q: `[[ -R ]]` said %q/%d and `[ -R ]` said %q/%d",
					src, name, a, sa, b, sb)
			}
		}
	}
}
