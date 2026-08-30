// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"strings"
	"testing"
)

// arith renders an expression as a fully parenthesised s-expression, so
// precedence and associativity are visible rather than implied.
func arith(e ArithExpr) string {
	switch x := e.(type) {
	case nil:
		return "<nil>"
	case *ArithNum:
		return x.Text
	case *ArithVar:
		return x.Name
	case *ArithUnary:
		if x.Postfix {
			return "(" + arith(x.X) + x.Op + ")"
		}
		return "(" + x.Op + arith(x.X) + ")"
	case *ArithBinary:
		return "(" + arith(x.X) + " " + x.Op + " " + arith(x.Y) + ")"
	case *ArithCond:
		return "(" + arith(x.Cond) + " ? " + arith(x.Then) + " : " + arith(x.Else) + ")"
	case *ArithAssign:
		return "(" + x.Name + " " + x.Op + " " + arith(x.Value) + ")"
	}
	return "?"
}

func parseArithOf(t *testing.T, src string, d Dialect) ArithExpr {
	t.Helper()
	f, err := Parse("echo $(("+src+"))", d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sc := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd)
	for _, w := range sc.Args {
		for _, s := range w.Spans {
			if s.Kind == ArithSubst {
				return s.Arith
			}
		}
	}
	t.Fatalf("no arithmetic in %q", src)
	return nil
}

func TestArithPrecedenceFollowsC(t *testing.T) {
	tests := []struct{ src, want string }{
		{`1+2*3`, `(1 + (2 * 3))`},
		{`(1+2)*3`, `((1 + 2) * 3)`},
		{`2*3%4`, `((2 * 3) % 4)`},
		{`1+2-3`, `((1 + 2) - 3)`},
		{`1<<2+3`, `(1 << (2 + 3))`},
		{`1|2^3&4`, `(1 | (2 ^ (3 & 4)))`},
		{`1==2!=3`, `((1 == 2) != 3)`},
		{`1<2==3`, `((1 < 2) == 3)`},
		{`1||2&&3`, `(1 || (2 && 3))`},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			if got := arith(parseArithOf(t, tc.src, Core())); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestArithShorterOperatorIsNotTakenOutOfALonger(t *testing.T) {
	// `<` must not be taken out of `<<` or `<=`, and `&` not out of `&&`.
	tests := []struct{ src, want string }{
		{`1<<2`, `(1 << 2)`},
		{`1<=2`, `(1 <= 2)`},
		{`1&&2`, `(1 && 2)`},
		{`1&2`, `(1 & 2)`},
		{`1>>2`, `(1 >> 2)`},
		{`1>=2`, `(1 >= 2)`},
	}
	for _, tc := range tests {
		if got := arith(parseArithOf(t, tc.src, Core())); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.src, got, tc.want)
		}
	}
}

func TestArithVariablesAndLiterals(t *testing.T) {
	// A bare name is a variable reference; both spellings mean the same.
	if got, want := arith(parseArithOf(t, `x+1`, Core())), `(x + 1)`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if got, want := arith(parseArithOf(t, `$x+1`, Core())), `(x + 1)`; got != want {
		t.Errorf("dollar form: got %s, want %s", got, want)
	}
	// A literal is kept as written: whether 0100 is sixty-four or one hundred
	// is a dialect question, so converting here would bake in an answer.
	for _, src := range []string{`0100`, `0x10`, `2#101`} {
		e := parseArithOf(t, src, Core())
		n, ok := e.(*ArithNum)
		if !ok || n.Text != src {
			t.Errorf("%s: literal not kept as written, got %s", src, arith(e))
		}
	}
}

func TestArithAssignmentIsRightAssociativeAndIsItsOwnNode(t *testing.T) {
	// Its effect outlives the expression, so it is not merely a binary
	// operator.
	if got, want := arith(parseArithOf(t, `x=y=1`, Core())), `(x = (y = 1))`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	for _, tc := range []struct{ src, want string }{
		{`x+=1`, `(x += 1)`},
		{`x<<=2`, `(x <<= 2)`},
	} {
		if got := arith(parseArithOf(t, tc.src, Core())); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.src, got, tc.want)
		}
	}
	// `==` is equality and must not be read as an assignment.
	if got, want := arith(parseArithOf(t, `x==1`, Core())), `(x == 1)`; got != want {
		t.Errorf("equality: got %s, want %s", got, want)
	}
}

func TestArithTernaryAndUnary(t *testing.T) {
	tests := []struct{ src, want string }{
		{`1?2:3`, `(1 ? 2 : 3)`},
		{`1?2:3?4:5`, `(1 ? 2 : (3 ? 4 : 5))`},
		{`-1+2`, `((-1) + 2)`},
		{`!0`, `(!0)`},
		{`~1`, `(~1)`},
		{`++x`, `(++x)`},
		{`x++`, `(x++)`},
	}
	for _, tc := range tests {
		if got := arith(parseArithOf(t, tc.src, Core())); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.src, got, tc.want)
		}
	}
}

func TestArithDialectGates(t *testing.T) {
	// dash has none of these, so posix must refuse them rather than accept
	// something it cannot mean.
	for _, src := range []string{`x++`, `1,2`, `2#101`} {
		if _, err := Parse("echo $(("+src+"))", POSIX()); err == nil {
			t.Errorf("%s: posix accepted a construct dash does not have", src)
		}
	}
	if got, want := arith(parseArithOf(t, `1,2`, Core())), `(1 , 2)`; got != want {
		t.Errorf("comma: got %s, want %s", got, want)
	}
}

func TestArithCommandIsParsedToo(t *testing.T) {
	f, err := Parse(`(( x = 2 > 1 ))`, Core())
	if err != nil {
		t.Fatal(err)
	}
	c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*ArithCmdClause)
	if c.Parsed == nil {
		t.Fatal("the (( )) command was not parsed")
	}
	if got, want := arith(c.Parsed), `(x = (2 > 1))`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestArithRejectsGarbageWithoutPanicking(t *testing.T) {
	for _, src := range []string{
		`1+`, `+`, `)`, `(`, `1?2`, `x=`, `1..2`, `,`, `**`, `1 2`,
		strings.Repeat("(", 100), strings.Repeat("1+", 200) + "1",
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on %q: %v", src, r)
				}
			}()
			// The error is the expected outcome for most of these; what is
			// being asserted is that it returns at all.
			_, _ = Parse("echo $(("+src+"))", Core())
		}()
	}
}
