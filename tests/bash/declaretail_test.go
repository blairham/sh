// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// The declaration long tail, measured against bash 5.3 (2026-09-04):
// `declare -f`/-F saying functions back, `local`'s option letters, the bare
// `local` and bare `set` listings, and `command -V`.

// bash lays a said-back function out with the name's line ending in ` () `,
// the brace alone on the next line with a trailing space, four-space
// indentation, and `;` after every statement but a block's last.
func TestDeclareFSaysTheFunctionBack(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `f() { if true; then echo one; fi; echo two; }
declare -f f`)
	want := "f () \n{ \n    if true; then\n        echo one;\n    fi;\n    echo two\n}\n"
	if out != want || st != 0 {
		t.Errorf("declare -f f = %q (status %d), want %q", out, st, want)
	}
}

// `declare -F` writes `declare -f name` per function with no operands, and
// the bare name once operands narrow it; a missing name is a silent 1.
func TestDeclareCapitalFNamesFunctions(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `f() { :; }
declare -F f
declare -F nosuch; echo st=$?`)
	if !strings.Contains(out, "f\n") || !strings.Contains(out, "st=1") || st != 0 {
		t.Errorf("got %q (status %d), want the bare name and the silent 1", out, st)
	}
}

// A bare `local` lists the innermost function's own locals as clustered
// declarations, attributes and all — measured: `declare -i n`, `declare -- x`.
func TestBareLocalListsTheFunctionsLocals(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `g() { local outer=1; f; }
f() { local x; local -i n; local; }
g`)
	want := "declare -i n\ndeclare -- x\n"
	if out != want || st != 0 {
		t.Errorf("bare local = %q (status %d), want %q", out, st, want)
	}
}

// `local` reads declare's letters here: `-i` evaluates the assignment,
// `-r` freezes the local, and `-g` reaches the global cell past the local.
func TestLocalReadsDeclareLetters(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `x=out
f() { local -i n; n=2+3; echo n=$n; local x=in; declare -g x=new; echo in=$x; }
f
echo out=$x`)
	for _, want := range []string{"n=5", "in=in", "out=new"} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want %q in it", out, want)
		}
	}
}

// A bad `local` name is refused with the operand quoted back whole, value
// and all, and the script carries on — bash alone is non-fatal here.
func TestLocalBadNameKeepsItsValue(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `f() { local 1x=5; echo st=$?; }; f; echo after`)
	if !strings.Contains(out, "local: `1x=5': not a valid identifier") {
		t.Errorf("got %q, want the operand quoted back with its value", out)
	}
	if !strings.Contains(out, "st=1") || !strings.Contains(out, "after") || st != 0 {
		t.Errorf("got %q (status %d), want status 1 and the script carrying on", out, st)
	}
}

// The case attributes: `-l` and `-u` fold the declaring assignment and every
// later one, and `-p` lists the letter after the others — `declare -irxl`.
func TestCaseAttributesFoldAndList(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `declare -l v=ABC
echo $v
v=DEF
echo $v
declare -p v`)
	want := "abc\ndef\ndeclare -l v=\"def\"\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A bare `set` lists the variables sorted — bare until a value needs
// quoting, `'\”` for an embedded quote — and then every defined function.
func TestBareSetListsVariablesThenFunctions(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `v1=plain
v2='has space'
v3="quo'te"
myfn() { echo hi; }
set`)
	if st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	for _, want := range []string{"v1=plain\n", "v2='has space'\n", `v3='quo'\''te'` + "\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want %q in it", out, want)
		}
	}
	if !strings.Contains(out, "myfn () \n{ \n    echo hi\n}\n") {
		t.Errorf("got %q, want the function after the variables", out)
	}
	if strings.Index(out, "v1=") > strings.Index(out, "myfn ()") {
		t.Errorf("got %q, want the variables before the functions", out)
	}
}

// `command -V` is `type`'s sentence with `command` on the complaint.
func TestCommandCapitalV(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `command -V echo
command -V if
command -V nosuch; echo st=$?`)
	for _, want := range []string{
		"echo is a shell builtin\n",
		"if is a shell keyword\n",
		"command: nosuch: not found",
		"st=1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want %q in it", out, want)
		}
	}
}

// `declare -gA` shares one implementation with `typeset -gA`, and shared it
// the bug too — measured against bash 5.3 on 2026-09-06. It was silent here
// rather than loud: this shell evaluates an unknown subscript as arithmetic,
// so a name that had lost the associative attribute took `m[k]=v` as index 0
// and `${m[k]}` read the same 0 back. The round trip looked right and
// `declare -p` was the witness — `declare -a m=([0]="v")` where the real
// shell says `declare -A m=([k]="v" )` (#989).
func TestGlobalAssociativeDeclaration(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the listing says associative, not indexed",
			"declare -gA m\nm[k]=v\ndeclare -p m", "declare -A m=([k]=\"v\" )\n",
		},
		{
			"the subscript is text, not arithmetic",
			"declare -gA m\nm[1+1]=x\ndeclare -p m", "declare -A m=([1+1]=\"x\" )\n",
		},
		{
			"the declaration outlives the function that made it",
			"f() { declare -gA m; m[k]=v; }\nf\ndeclare -p m", "declare -A m=([k]=\"v\" )\n",
		},
		{
			"a valueless global declaration still records the attribute",
			"f() { declare -gA m; }\nf\ndeclare -p m", "declare -A m\n",
		},
		{
			"the letters in either order",
			"declare -Ag m\nm[k]=v\necho \"[${m[k]}]\"", "[v]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}

// `-i` takes no output base in this shell, which is what separates it from
// the two that have `integer` — and there is no `integer` here at all.
// Measured 2026-09-06 on bash 5.3.15, `env -i PATH=/usr/bin:/bin` with a
// scratch HOME, over a script file:
//
//	typeset -i2 c=5        -2: invalid option, st=2
//	typeset -i 16 b=255    `16': not a valid identifier, st=1
//	integer n=3            integer: command not found, st=127
//
// The axis is asked only where a base is written, so the answer matters
// exactly here: with it left unanswered, `typeset -i2` reported that the
// shells disagree and no dialect had been chosen — when one had.
func TestTheIntegerLetterTakesNoBaseHere(t *testing.T) {
	if got := bash.Semantics().IntegerAttributeTakesABase; got != interp.No {
		t.Errorf("IntegerAttributeTakesABase = %v, want No", got)
	}
	if got := bash.Semantics().IntegerOptions; got != "" {
		t.Errorf("IntegerOptions = %q, want empty — there is no `integer` here", got)
	}
	out, st := runBash(t, t.TempDir(), `typeset -i2 c=5`)
	if !strings.Contains(out, "typeset: -2: invalid option") || st != 2 {
		t.Errorf("typeset -i2 = %q (status %d), want the letter refused as invalid "+
			"and nothing said about a base", out, st)
	}
	if strings.Contains(out, "output base") || strings.Contains(out, "no dialect was chosen") {
		t.Errorf("typeset -i2 = %q, want no word about a base in a shell whose `-i` "+
			"takes none", out)
	}
	out, st = runBash(t, t.TempDir(), `integer n=3; echo "st=$?"`)
	if !strings.Contains(out, "integer: command not found") || !strings.Contains(out, "st=127") {
		t.Errorf("integer n=3 = %q (status %d), want a command that is not found", out, st)
	}
}
