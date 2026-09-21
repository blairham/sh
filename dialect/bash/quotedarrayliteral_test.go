// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// A declaration utility's value that came out as `( … )` is read again as an
// array literal here, and a `let` operand written as a compound assignment
// reaches the builtin as one word. Both are this column's answer alone: see
// Semantics.DeclarationRereadsAParenthesizedValue and the `let` entry in
// Dialect.DeclarationUtilities for the panel rows (#2298).
//
// Measured 2026-09-21 on bash 5.3.20, each line its own `-c` under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`.

func runQuotedLiteral(t *testing.T, src string) (string, string, int) {
	t.Helper()
	var out, errs bytes.Buffer
	code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", src})
	return out.String(), errs.String(), code
}

func TestADeclarationReadsAParenthesizedValueAgain(t *testing.T) {
	for _, c := range []struct {
		name, src, out string
	}{
		// The letter on this line is enough to make the name an array, and
		// the elements are counted from it.
		{"the array letter", `declare -a a="(hello world)"; declare -p a`, `declare -a a=([0]="hello" [1]="world")` + "\n"},
		// A standing attribute is enough too: the letter need not be on the
		// line that carries the value.
		{"a standing attribute", `declare -a a; declare a="(1 2)"; declare -p a`, `declare -a a=([0]="1" [1]="2")` + "\n"},
		// The table letter keys them instead, through the same store the
		// written literal uses.
		{"the table letter", `declare -A m="([k]=v)"; declare -p m`, `declare -A m=([k]="v" )` + "\n"},
		// The elements are expanded where they stand rather than split out
		// of the text: an integer attribute folds them.
		{"the integer letter folds", `declare -ai n="(1+1 2*2)"; declare -p n`, `declare -ai n=([0]="2" [1]="4")` + "\n"},
		// And a parameter inside splits, exactly as it does in a written
		// literal. This is the row that says the text is re-read rather than
		// cut at the blanks.
		{"a parameter splits", `x="a b"; declare -a d="($x)"; declare -p d`, `declare -a d=([0]="a" [1]="b")` + "\n"},
		{"a substitution runs", `declare -a e="($(echo one two))"; declare -p e`, `declare -a e=([0]="one" [1]="two")` + "\n"},
		// The appending spelling appends, rather than starting the name over.
		{"an append joins", `declare -a a=(x); declare a+="(y z)"; declare -p a`, `declare -a a=([0]="x" [1]="y" [2]="z")` + "\n"},
		// `local` has a loop of its own, so the rule has to be asked there
		// too — the row that fails when only one of the two is fixed.
		{"local reads it too", `f(){ local -a q="(1 2)"; declare -p q; }; f`, `declare -a q=([0]="1" [1]="2")` + "\n"},
		// Four shapes that are *not* a literal, and each is a measurement.
		{"no declaration word", `declare -a a; a="(1 2)"; declare -p a`, `declare -a a=([0]="(1 2)")` + "\n"},
		{"no array attribute", `declare a="(1 2)"; declare -p a`, `declare -- a="(1 2)"` + "\n"},
		{"a subscripted operand", `declare -a a; declare a[1]="(var)"; declare -p a`, `declare -a a=([1]="(var)")` + "\n"},
		{"a space outside", `declare -a x=" (a b) "; declare -p x`, `declare -a x=([0]=" (a b) ")` + "\n"},
		{"no closing parenthesis", `declare -a x="(a b"; declare -p x`, `declare -a x=([0]="(a b")` + "\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runQuotedLiteral(t, c.src)
			if out != c.out || errs != "" || code != 0 {
				t.Errorf("%s\n got %q %q %d\nwant %q %q 0", c.src, out, errs, code, c.out, "")
			}
		})
	}
}

func TestLetTakesACompoundAssignmentOperand(t *testing.T) {
	for _, c := range []struct {
		name, src, out, errs string
	}{
		// The parenthesized expression, and the reason this is not worth one
		// line: before the word was on the list the line was a *parse*
		// failure, which ends the script.
		{"a parenthesized expression", `let x=(2 + 3); echo "x=$x"`, "x=5\n", ""},
		{"two operands", `let x=(2 + 3) y=(4 + 7); echo "$x $y"`, "5 11\n", ""},
		{"the script runs on", `let x=(1); echo tail`, "tail\n", ""},
		{"a parameter inside", `v=3; let a=($v + 1); echo "a=$a"`, "a=4\n", ""},
		{"an append", `a=1; let a+=(4); echo "a=$a"`, "a=5\n", ""},
		// What reaches the builtin is one word holding the assignment, so a
		// blank inside it is the arithmetic reader's problem rather than a
		// second operand: the complaint quotes the whole word back.
		{
			"a blank inside is one word", `let a=(1 2); echo "a=$a st=$?"`,
			"a= st=1\n",
			"bash: line 1: let: a=(1 2): missing `)' (error token is \"2)\")\n",
		},
		{
			"an empty literal", `let a=(); echo "st=$?"`,
			"st=1\n",
			"bash: line 1: let: a=(): arithmetic syntax error: operand expected (error token is \")\")\n",
		},
		// And it is a fact about the *word*, not about what the word resolves
		// to: `command let` is a syntax error in bash as it is here.
		{
			"command let is not the word", `command let a=(1)`,
			"",
			"bash: -c: line 1: syntax error near unexpected token `('\nbash: -c: line 1: `command let a=(1)'\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, _ := runQuotedLiteral(t, c.src)
			if out != c.out || errs != c.errs {
				t.Errorf("%s\n got %q %q\nwant %q %q", c.src, out, errs, c.out, c.errs)
			}
		})
	}
}
