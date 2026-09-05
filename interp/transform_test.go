// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// The `${x@Q}` transformation family. Every expectation here is a measured
// fact recorded in docs/spec/grammar/parameter-expansion.md; the tests name
// the construct and the grammar flag, and the dialect preset is only the
// harness that turns the flag on.

// runTransform is run() with the grammar flags the family needs, named for
// the constructs: the transformations themselves, and the case-change
// operator a few rows contrast them with.
func runTransform(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamTransformations = true
		d.ParamCaseChange = true
		// One row reaches a transformation through `${!p…}`.
		d.ParamIndirection = true
	}, nil)
}

func TestTransformQuotesForReuse(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// Single quotes even when nothing needs them, the quote itself
		// spelled '\'', and a backslash left alone.
		{"plain", `x=plain; echo "${x@Q}"`, `'plain'`},
		{"a space", `x="a b"; echo "${x@Q}"`, `'a b'`},
		{"an embedded quote", `x="a b'c"; echo "${x@Q}"`, `'a b'\''c'`},
		{"a backslash stays single-quoted", `x='has\backslash'; echo "${x@Q}"`, `'has\backslash'`},
		{"dollar and double quote too", `x='d$q"w'; echo "${x@Q}"`, `'d$q"w'`},
		{"empty is a quoted nothing", `x=; echo "[${x@Q}]"`, `['']`},
		{"unset is nothing at all", `echo "[${u@Q}]"`, `[]`},

		// Any control character switches the whole value to $'…'.
		{"a tab", `x=$'a\tb'; echo "${x@Q}"`, `$'a\tb'`},
		{"a newline", `x=$'\n'; echo "${x@Q}"`, `$'\n'`},
		{"the named escapes", `x=$'a\ab\bc\fd\ve'; echo "${x@Q}"`, `$'a\ab\bc\fd\ve'`},
		{"escape is E", `x=$'a\x1bb'; echo "${x@Q}"`, `$'a\Eb'`},
		{"other controls are octal", `x=$'a\x01b'; echo "${x@Q}"`, `$'a\001b'`},
		{"delete is octal", `x=$'\x7f'; echo "${x@Q}"`, `$'\177'`},
		{"a quote inside the dollar form", `x=$'it\x27s\ttab'; echo "${x@Q}"`, `$'it\'s\ttab'`},
		{"a backslash inside the dollar form", `x=$'back\\slash\t'; echo "${x@Q}"`, `$'back\\slash\t'`},
		// Printable multibyte text is not what forces the switch.
		{"multibyte stays plain", `x=café; echo "${x@Q}"`, `'café'`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runTransform(t, c.src)
			if strings.TrimSpace(out) != c.want || st != 0 {
				t.Errorf("said %q st=%d, want %q st=0", strings.TrimSpace(out), st, c.want)
			}
		})
	}
}

func TestTransformExpandsEscapes(t *testing.T) {
	// @E reads the value under the $'…' rules: the same decoder, so the two
	// cannot drift apart.
	out, _ := runTransform(t, `x='a\tb'; echo "${x@E}"`)
	if out != "a\tb\n" {
		t.Errorf("@E said %q, want a real tab", out)
	}
	out, _ = runTransform(t, `x='oct\101 hex\x41'; echo "${x@E}"`)
	if out != "octA hexA\n" {
		t.Errorf("@E said %q, want the escapes decoded", out)
	}
	out, _ = runTransform(t, `echo "[${u@E}]"`)
	if out != "[]\n" {
		t.Errorf("@E of unset said %q, want empty", out)
	}
}

func TestTransformChangesCase(t *testing.T) {
	out, _ := runTransform(t, `x="abC dEf"; echo "${x@U}|${x@L}|${x@u}"`)
	if strings.TrimSpace(out) != "ABC DEF|abc def|AbC dEf" {
		t.Errorf("case letters said %q", strings.TrimSpace(out))
	}
	// Unset is empty for all three, and an empty value stays empty.
	out, _ = runTransform(t, `echo "[${u@U}][${u@L}][${u@u}]"`)
	if strings.TrimSpace(out) != "[][][]" {
		t.Errorf("case letters on unset said %q", strings.TrimSpace(out))
	}
}

func TestTransformReportsAttributes(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"integer", `typeset -i n=1; echo "[${n@a}]"`, `[i]`},
		{"none", `x=plain; echo "[${x@a}]"`, `[]`},
		{"unset", `echo "[${u@a}]"`, `[]`},
		{"readonly and export, ordered", `typeset -rx v=1; echo "[${v@a}]"`, `[rx]`},
		{"all three, ordered", `typeset -irx v=1; echo "[${v@a}]"`, `[irx]`},
		{"an array", `typeset -a arr=(1 2); echo "[${arr@a}]"`, `[a]`},
		{"an associative array", `typeset -A h; h[k]=1; echo "[${h@a}]"`, `[A]`},
		// The attributes belong to the name, so every element answers alike.
		{"per element", `a=(x y); echo "${a[@]@a}"`, `a a`},
		// An indirection reports the target's attributes, not the pointer's.
		{"through an indirection", `typeset -i tgt=1; p=tgt; echo "[${!p@a}]"`, `[i]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runTransform(t, c.src)
			if strings.TrimSpace(out) != c.want {
				t.Errorf("said %q, want %q", strings.TrimSpace(out), c.want)
			}
		})
	}
}

func TestTransformWritesAnAssignment(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// A bare name is name='value'; attributes put a declare in front.
		{"a scalar", `x="a b"; echo "${x@A}"`, `x='a b'`},
		{"with attributes", `typeset -irx v=1; echo "${v@A}"`, `declare -irx v='1'`},
		{"unset is nothing", `echo "[${u@A}]"`, `[]`},
		{"a positional has no name", `set -- pq; echo "[${1@A}]"`, `[]`},
		// The whole-array form is the words of the statement, one field each.
		{
			"an array", `a=(1 "x y"); printf "[%s]" "${a[@]@A}"; echo`,
			`[declare][-a][a=([0]="1" [1]="x y")]`,
		},
		{
			"an array joined", `a=(one "t w"); printf "[%s]" "${a[*]@A}"; echo`,
			`[declare -a a=([0]="one" [1]="t w")]`,
		},
		{
			"element quoting", `a=('say "hi"'); printf "[%s]" "${a[@]@A}"; echo`,
			`[declare][-a][a=([0]="say \"hi\"")]`,
		},
		// An associative table's list carries its trailing space, measured.
		{
			"an associative array", `typeset -A h; h[k]="v 1"; printf "[%s]" "${h[@]@A}"; echo`,
			`[declare][-A][h=([k]="v 1" )]`,
		},
		// The positional parameters come back as the set command that would
		// restore them, and as nothing when there are none to restore.
		{
			"positionals", `set -- one "t w"; printf "[%s]" "${@@A}"; echo`,
			`[set][--]['one']['t w']`,
		},
		{"no positionals", `set --; printf "[%s]" "${@@A}"; echo`, `[]`},
		// The scalar view of an array is the first element, reproduced with
		// the array's attribute — the measured quirk, not an invention.
		{"an array read as a scalar", `a=(1 "x y"); echo "${a@A}"`, `declare -a a='1'`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runTransform(t, c.src)
			if strings.TrimSpace(out) != c.want {
				t.Errorf("said %q, want %q", strings.TrimSpace(out), c.want)
			}
		})
	}
}

func TestTransformListsKeysAndValues(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// @K is one word, keys bare and values double-quoted; @k is the same
		// pairs as separate words, values raw.
		{
			"K is one field", `a=(one "t w"); printf "[%s]" "${a[@]@K}"; echo`,
			`[0 "one" 1 "t w"]`,
		},
		{
			"k is separate fields", `a=(one "t w"); printf "[%s]" "${a[@]@k}"; echo`,
			`[0][one][1][t w]`,
		},
		{
			"K escapes its values", `a=('say "hi"' 'back\slash'); echo "${a[@]@K}"`,
			`0 "say \"hi\"" 1 "back\\slash"`,
		},
		{
			"a control character forces the dollar form", `a=($'t\tb'); echo "${a[@]@K}"`,
			`0 $'t\tb'`,
		},
		{"k joined", `a=(one "t w"); printf "[%s]" "${a[*]@k}"; echo`, `[0 one 1 t w]`},
		// An associative table answers in key order — this implementation's
		// everywhere — with the measured trailing space per pair.
		{
			"an associative array", `typeset -A h; h[k]="v 1"; printf "[%s]" "${h[@]@K}"; echo`,
			`[k "v 1" ]`,
		},
		{
			"associative keys sorted", `typeset -A h; h[y]=3; h[x]="1 2"; echo "${h[@]@k}"`,
			`x 1 2 y 3`,
		},
		// Anything that is not a stored array answers as @Q: quoted values,
		// no keys — a scalar, a scalar read as `${x[@]}`, the positionals.
		{"a scalar", `x="a b"; echo "${x@K}"; echo "${x@k}"`, "'a b'\n'a b'"},
		{"a scalar read as an array", `x=abc; echo "${x[@]@k}"`, `'abc'`},
		{
			"positionals have no keys", `set -- one "t w"; printf "[%s]" "${@@K}"; echo`,
			`['one']['t w']`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runTransform(t, c.src)
			if strings.TrimSpace(out) != c.want {
				t.Errorf("said %q, want %q", strings.TrimSpace(out), c.want)
			}
		})
	}
}

func TestTransformDistributesOverAWholeArray(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// One transformed word per element for [@], joined for [*] — the
		// same split "$@" and "$*" already have.
		{
			"quoted elements", `a=(one "t w" "q't"); printf "[%s]" "${a[@]@Q}"; echo`,
			`['one']['t w']['q'\''t']`,
		},
		{
			"joined elements", `a=(one "t w"); printf "[%s]" "${a[*]@Q}"; echo`,
			`['one' 't w']`,
		},
		{"case per element", `a=(ab "c d"); printf "[%s]" "${a[@]@U}"; echo`, `[AB][C D]`},
		{
			"escapes per element", `a=('x\ty' b); printf "[%s]" "${a[@]@E}"; echo`,
			"[x\ty][b]",
		},
		{"an unset array is zero fields", `unset a; printf "[%s]" "${a[@]@Q}"; echo`, `[]`},
		{"an empty array is zero fields", `a=(); printf "[%s]" "${a[@]@Q}"; echo`, `[]`},
		{"one element is the scalar path", `a=(one two); echo "${a[1]@Q}"`, `'two'`},
		// The positional parameters distribute the same way.
		{"positionals", `set -- one "t w"; printf "[%s]" "${@@Q}"; echo`, `['one']['t w']`},
		{
			"positionals joined", `set -- one "t w"; printf "[%s]" "${*@Q}"; echo`,
			`['one' 't w']`,
		},
		{"no positionals", `set --; printf "[%s]" "${@@Q}"; echo`, `[]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runTransform(t, c.src)
			if strings.TrimSpace(out) != c.want {
				t.Errorf("said %q, want %q", strings.TrimSpace(out), c.want)
			}
		})
	}
}

// @P is prompt expansion, which needs machinery the interpreter does not
// hold. The refusal is loud, names the letter, and stops the command — a
// value with no prompt escapes would pass through unchanged, but answering
// only that case would be a silent wrong answer for every other value.
func TestTransformRefusesPromptExpansion(t *testing.T) {
	out, st := runTransform(t, `x=abc; echo "hi ${x@P}"; echo after`)
	if st == 0 {
		t.Error("the @P refusal reported success")
	}
	if !strings.Contains(out, "@P") {
		t.Errorf("the refusal %q does not name the letter", out)
	}
	if strings.Contains(out, "hi") || strings.Contains(out, "after") {
		t.Errorf("output %q: the command ran despite the refusal", out)
	}
}

// A letter outside the set never became an operator, so it reports as the
// deferred bad substitution it stayed.
func TestUnknownTransformLetterIsABadSubstitution(t *testing.T) {
	out, st := runTransform(t, `x=abc; echo "${x@Z}"`)
	if st == 0 {
		t.Error("an unknown letter reported success")
	}
	if !strings.Contains(out, "${x@Z}") || !strings.Contains(out, "bad substitution") {
		t.Errorf("said %q, want the construct named as a bad substitution", out)
	}
}

// The grammar that refuses other bad operators while reading defers the `@`
// family, and its runtime report has a wording of its own — the parse-time
// wording is a syntax error that would misreport what happened here.
func TestDeferredBadSubstitutionUsesItsRunWording(t *testing.T) {
	d := syntax.Core()
	d.BadSubstitutionAtParseTime = true
	f, err := syntax.Parse(`echo "${x@Q}"`, d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sem := permissive()
	sem.FatalErrorStatusIsOne = Yes
	dg := Diagnostics{
		BadSubstitution:      "syntax error: `%[1]s' unexpected",
		BadSubstitutionAtRun: "${%[1]s}: bad substitution",
	}
	var buf bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	if !strings.Contains(buf.String(), "${x@Q}: bad substitution") {
		t.Errorf("said %q, want the run wording", buf.String())
	}
	if strings.Contains(buf.String(), "syntax error") {
		t.Errorf("said %q, want the parse wording left unused", buf.String())
	}
	if st != 1 {
		t.Errorf("status %d, want the fatal-error status", st)
	}
}
