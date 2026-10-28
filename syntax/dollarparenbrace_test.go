// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// Dialect.CloseBraceEndsADollarDoubleParen: a `}` that closes no `${` ends the
// `$((` it stands in, before the construct is read as arithmetic or as a
// command substitution. What was opened expands to nothing, and the text behind
// the brace is the rest of the line — so what is refused is that text's own
// first stray token, and where nothing is wrong with it the line runs with an
// empty expansion where the construct was. Measured 2026-10-27 on ksh93u+
// 2012-08-01 (#6918).
func TestACloseBraceEndsADollarDoubleParen(t *testing.T) {
	t.Parallel()
	ending := syntax.Core()
	ending.CloseBraceEndsADollarDoubleParen = true

	// What stands behind the brace is refused by its own token.
	for _, tc := range []struct{ src, token string }{
		{"echo $(( exec {v}</dev/null; echo $v ) | cat)", ")"},
		{"echo $(( echo } ) | cat)", ")"},
		{"echo $(( } ))", ")"},
		{"echo $(( echo }; fi ) | cat)", "fi"},
		{"echo $(( echo ${v}} ) | cat)", ")"},
		{"echo $(( echo ${v:-{a}} ) | cat)", ")"},
		{`echo $(( echo ${v:-"x"}} ) | cat)`, ")"},
		{`echo $(( echo \\} ) | cat)`, ")"},
		{"echo $(( echo a #}\n) | cat)", ")"},
	} {
		_, err := syntax.Parse(tc.src, ending)
		var se *syntax.Error
		if !errors.As(err, &se) || se.Kind != syntax.ErrUnexpected || se.Token != tc.token {
			t.Errorf("%q: err %v, want `%s' unexpected", tc.src, err, tc.token)
		}
		if _, err := syntax.Parse(tc.src, syntax.Core()); err != nil {
			t.Errorf("%q without the flag: %v, want the construct read whole", tc.src, err)
		}
	}

	// Where nothing behind the brace is wrong, the line parses with an empty
	// command substitution in the construct's place, and the rest of the text
	// read on from the brace.
	for _, tc := range []struct {
		src   string
		words []string // the printed words after `echo`, or the assignment
	}{
		{"echo $(( echo }", []string{"$()"}},
		{"echo $(( } fi", []string{"$()", "fi"}},
		{`x="$(( } ))"`, []string{`x="$() ))"`}},
		{`echo $(( echo "}" ) | cat); echo "after"`, []string{`$()" ) | cat); echo "after`, `"`}},
		{"echo $((\necho }\n)", []string{"$()", ")"}},
	} {
		f, err := syntax.Parse(tc.src, ending)
		if err != nil {
			// What is left over is refused, and the two rows that end in a
			// token say what: a `)` on line 3, where the brace ended the
			// construct on line 2; and a quote the end of the input leaves
			// open, because the one that opened before the brace went with
			// the construct. The shells run the second under their leniency
			// about a `-c` string's last quote, which the strict parser has
			// no part in.
			var se *syntax.Error
			switch last := tc.words[len(tc.words)-1]; last {
			case ")":
				if !errors.As(err, &se) || se.Token != ")" || se.Pos.Line != 3 {
					t.Errorf("%q: %v, want `)' unexpected on line 3", tc.src, err)
				}
			case `"`:
				if !errors.As(err, &se) || !strings.Contains(se.Error(), "unterminated double quote") ||
					se.Pos.Col != 40 {
					t.Errorf("%q: %v, want the last quote unterminated", tc.src, err)
				}
			default:
				t.Errorf("%q: %v", tc.src, err)
			}
			continue
		}
		cmd, ok := onlyCommand(t, tc.src, ending).(*syntax.SimpleCmd)
		if !ok {
			t.Fatalf("%q is %T, want a simple command", tc.src, f.Stmts[0].Expr)
		}
		var got []string
		var first *syntax.Word
		for _, a := range cmd.Assigns {
			got = append(got, a.Name+"="+syntax.PrintWord(a.Value))
			first = a.Value
		}
		for _, w := range cmd.Args[min(len(cmd.Args), 1):] {
			got = append(got, syntax.PrintWord(w))
			if first == nil {
				first = w
			}
		}
		if len(got) != len(tc.words) {
			t.Fatalf("%q read %q, want %q", tc.src, got, tc.words)
		}
		for i := range got {
			if got[i] != tc.words[i] {
				t.Errorf("%q word %d is %q, want %q", tc.src, i, got[i], tc.words[i])
			}
		}
		if s := first.Spans[0]; s.Kind != syntax.CommandSubst || s.Value != "" {
			t.Errorf("%q: the construct is %v %q, want an empty command substitution", tc.src, s.Kind, s.Value)
		}
	}

	// The controls: a brace that is shielded, that closes a `${`, or that
	// stands outside the construct leaves the reading the core has.
	for _, src := range []string{
		"echo $(( 1 ))",
		"echo $(( 1 )) }",
		`echo $(( echo \} ) | cat)`,
		`echo $(( echo '}' ) | cat)`,
		"echo $(( echo `echo }` ) | cat)",
		"echo $(( echo $(echo a}) ) | cat)",
		"echo $(( echo ${v}x ) | cat)",
		"echo $(( echo ${ echo a;} ) | cat)",
		`echo $(( echo ${v/\}/x} ) | cat)`,
		`echo $(( echo "${v:-{a}}" ) | cat)`,
		`echo $(( echo ${v:-"}"} ) | cat)`,
		`echo $(( echo ${v:-"${w}}"} ) | cat)`,
		"echo $(( echo {a ) | cat)",
		`echo "$(( 1 ))"`,
	} {
		with, err := syntax.Parse(src, ending)
		if err != nil {
			t.Errorf("%q: %v, want it parsed", src, err)
			continue
		}
		without, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatalf("%q without the flag: %v", src, err)
		}
		if why, same := syntax.SameProgram(with, without); !same {
			t.Errorf("%q reads differently with the flag: %s", src, why)
		}
	}
}
