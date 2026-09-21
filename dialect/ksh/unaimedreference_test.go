// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A read or a write **through** a reference with nothing to point at is
// refused, where the two other shells that spell a reference answer with the
// empty string.
//
// Measured 2026-09-20 against AT&T ksh93u+ 2012-08-01 (`/bin/ksh` here),
// `-c` under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the
// null device, every row over `typeset -n u`. Each of these answered empty
// at status 0 here, which reads exactly like a reference aimed at a name
// holding nothing — and `typeset -n out; some_fn out` before the call is an
// ordinary way for a script to be in that state (#3955).
//
// The refusal is ahead of the operators, which is measured rather than
// assumed: the word behind a `-` does not stand in for the value. That pair
// is the one shape the scalar expansion never sees, because the word is
// expanded before the parameter is read.
func TestAReadThroughAnUnaimedReferenceIsRefused(t *testing.T) {
	t.Parallel()
	const u = `typeset -n u; `
	for _, c := range []struct{ name, src string }{
		{"the plain read", u + `print -r -- "[${u}]"; print after`},
		{"a member path, which is a use of its base", u + `print -r -- "[${u.a}]"; print after`},
		{"the length", u + `print -r -- "[${#u}]"; print after`},
		{"a default that would have fired", u + `print -r -- "[${u:-D}]"; print after`},
		{"and the colonless spelling of it", u + `print -r -- "[${u-D}]"; print after`},
		{"an alternate that would not have", u + `print -r -- "[${u+SET}]"; print after`},
		{"the error operator", u + `print -r -- "[${u:?msg}]"; print after`},
		{"a trim", u + `print -r -- "[${u#x}]"; print after`},
		{"a replacement", u + `print -r -- "[${u/a/b}]"; print after`},
		{"a substring", u + `print -r -- "[${u:1:2}]"; print after`},
		{"a write to one of its members", u + `u.a=5; print after`},
		// The contexts that reach the scalar expansion **directly** rather
		// than through a span, which is the second call the rule needs: a
		// pattern operand, a here-document's body and a nested expansion
		// each arrive having never seen the span path. Removing either call
		// leaves rows in this list failing, which is checked rather than
		// assumed.
		{"a case pattern", u + `case x in ${u}) print m;; *) print n;; esac; print after`},
		{"a replacement's pattern", u + `print -r -- "${x/${u}/y}"; print after`},
		{"an expansion nested in another", u + `print -r -- "${x:-${u}}"; print after`},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := answersRun(t, c.src)
			if out != "sh: u: no reference name\n" || status != 1 {
				t.Errorf("%s = %q at %d, want the refusal at 1", c.src, out, status)
			}
		})
	}
}

// `unset` says the same thing and does not end the script, which is the one
// route of the three whose cost differs.
func TestAnUnsetThroughAnUnaimedReferenceIsRefusedAndTheScriptGoesOn(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src string }{
		{"the name itself", `typeset -n u; unset u; print "st=$? after"`},
		{"and one of its members", `typeset -n u; unset u.a; print "st=$? after"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := answersRun(t, c.src)
			const want = "sh: unset: u: no reference name\nst=1 after\n"
			if out != want || status != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", c.src, out, status, want)
			}
		})
	}
}

// And the states that are **not** a use of the reference, every one of which
// must stay silent.
//
// These are what keep the refusal from widening into "an unaimed reference is
// refused": a declaration aiming one is fine, a value that aims it is fine,
// and asking *about* the reference rather than through it is fine. Each was
// already right and each is a row a mutation of the check above moves.
func TestAnUnaimedReferenceIsOnlyRefusedWhenSomethingGoesThroughIt(t *testing.T) {
	t.Parallel()
	const u = `typeset -n u; `
	for _, c := range []struct{ name, src, want string }{
		{"a listing names it", u + `typeset -p u; print after`, "typeset -n u\nafter\n"},
		{"the is-it-set test", u + `[[ -v u ]]; print "v=$? after"`, "v=1 after\n"},
		{"a prefix listing", u + `print -r -- "[${!u@}]"; print after`, "[]\nafter\n"},
		{"a loop re-aims it", u + `for u in a b; do print -r -- "[$u]"; done; print after`, "[]\n[]\nafter\n"},
		{"a scalar aims it", u + `typeset u=plain; print -r -- "st=$? [${u}] after"`, "st=0 [] after\n"},
		{"an array literal takes its name", u + `typeset u=(1 2); print -r -- "st=$? [${u[@]}] after"`, "st=0 [1 2] after\n"},
		{"removing the letter", u + `unset -n u; print "st=$? after"`, "st=0 after\n"},
		// And the control that says none of this is about references in
		// general: an aimed one reads and writes as it always did.
		{
			"the control: an aimed reference",
			`v=1; typeset -n r=v; r=2; print -r -- "[${r}][${r-D}][${v}]"`,
			"[2][2][2]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := answersRun(t, c.src)
			if out != c.want || status != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", c.src, out, status, c.want)
			}
		})
	}
}

// A here-document's body is the fourth direct reach, and the one route where
// the refusal does not end the script: the document is written, the shell
// says so, and the next line runs. Measured, and ksh93u+ agrees row for row.
func TestAHereDocumentThroughAnUnaimedReferenceIsRefused(t *testing.T) {
	t.Parallel()
	src := "typeset -n u\ncat <<EOF\n[${u}]\nEOF\nprint after\n"
	out, status := answersRun(t, src)
	const want = "sh: line 2: u: no reference name\nafter\n"
	if out != want || status != 0 {
		t.Errorf("a here-document = %q at %d, want %q at 0", out, status, want)
	}
}

// Two rows of the family are **recorded rather than reproduced**, and both
// are about what the refusal costs rather than about whether it is made.
//
// A conditional's pattern operand ends the script in ksh93u+ and is reported
// and carried on from here, and a subscript that reads through the reference
// writes the sentence twice here where the reference writes it once. Both
// are the expansion-failure machinery either side of this rule — the one
// FailedExpansionAbandonsTheLine answers, and the double read of a subscript
// — rather than anything this refusal decides, and pinning them keeps a
// later change to either deliberate.
func TestWhatAnUnaimedReferenceCostsTwoOtherContexts(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			"a conditional's pattern operand: the script carries on here",
			`typeset -n u; [[ x == ${u} ]]; print "t=$? after"`,
			"sh: u: no reference name\nt=1 after\n", 0,
		},
		{
			"a subscript: the sentence is written twice here",
			`typeset -n u; a=(1 2); print -r -- "${a[${u}]}"; print after`,
			"sh: u: no reference name\nsh: u: no reference name\n", 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := answersRun(t, c.src)
			if out != c.want || status != c.status {
				t.Errorf("%s = %q at %d, want %q at %d", c.src, out, status, c.want, c.status)
			}
		})
	}
}
