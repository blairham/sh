// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `readonly -a q="(1 2)"` is the same operand `typeset -a q="(1 2)"` carries:
// the quoting hid the parentheses from the parser and the letter on the line
// says the name is an array, so the text is read again as the literal it was
// written as.
//
// The letter has to be **on this line** for this word, which is the half that
// separates it from `declare` — measured 2026-09-22 on bash 5.3.20 and in the
// rows below.
func TestReadonlyReadsAQuotedArrayLiteralAgain(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ src, want string }{
		{`readonly -a d="(1 2)"; declare -p d`, `declare -ar d=([0]="1" [1]="2")`},
		{`readonly -A m="([k]=v)"; declare -p m`, `declare -Ar m=([k]="v" )`},
		// A value with the letter and no parentheses: the letter still names
		// the kind, which the axis for the *valueless* form does not.
		{`readonly -a d=4; declare -p d`, `declare -ar d=([0]="4")`},
		// And with no value at all the letter records nothing here, which is
		// Semantics.ReadonlyRecordsTheCompoundAttribute and is unchanged.
		{`readonly -a d; declare -p d`, `declare -r d`},
		// The standing attribute is not enough for this word. `declare` and
		// `typeset` re-read on it and `readonly` and `export` do not.
		{`declare -a c; readonly c="(3)"; declare -p c`, `declare -ar c=([0]="(3)")`},
		{`declare -a c; declare c="(3)"; declare -p c`, `declare -a c=([0]="3")`},
		// A bare assignment never re-reads, whatever the name is.
		{`declare -a c; c="(3)"; declare -p c`, `declare -a c=([0]="(3)")`},
	} {
		out, st := runBashPrelude(t, dir, c.src)
		if st != 0 {
			t.Errorf("%q: status %d, output %q", c.src, st, out)
		}
		wantWholeLines(t, out, c.want)
	}
}

// And a frozen name is refused **once**, before anything is written.
//
// Everything the operand would otherwise do writes: the kind mark converts a
// standing scalar and the store lays the elements down, and each reports on
// its own once it has already written. Measured on bash 5.3.20 — one
// sentence, the builtin named, and the array exactly as it was.
func TestAFrozenNameRefusesAHiddenLiteralOnce(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ src, refusal, left string }{
		{
			"a=(1); readonly a\nreadonly -a a=\"(4)\"\ndeclare -p a",
			"bash: line 3: readonly: a: readonly variable", `declare -ar a=([0]="1")`,
		},
		{
			"a=1; readonly a\nreadonly -a a=\"(4)\"\ndeclare -p a",
			"bash: line 3: readonly: a: readonly variable", `declare -r a="1"`,
		},
		{
			"declare -r a\nreadonly -a a=\"(4)\"\ndeclare -p a",
			"bash: line 3: readonly: a: readonly variable", `declare -r a`,
		},
		// The written literal is the control and stays bare: that store is
		// the command's rather than the builtin's own reading.
		{
			"a=(1); readonly a\nreadonly -a a=(4)\ndeclare -p a",
			"bash: line 3: a: readonly variable", `declare -ar a=([0]="1")`,
		},
		// And so is a plain value under this word.
		{
			"a=(1); readonly a\nreadonly a=4\ndeclare -p a",
			"bash: line 3: a: readonly variable", `declare -ar a=([0]="1")`,
		},
	} {
		out, _ := runBashPrelude(t, dir, "\n"+c.src)
		wantWholeLines(t, out, c.refusal, c.left)
		if n := countLines(out, c.refusal); n != 1 {
			t.Errorf("%q: the refusal appears %d times in %q, want once", c.src, n, out)
		}
	}
}
