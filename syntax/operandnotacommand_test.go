// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// An expansion's operand is not a command position, so the `((` that opens an
// arithmetic command is two ordinary characters in one.
//
// Measured 2026-09-07 and **unanimous** across bash 5.3.15, bash 3.2.57, that
// build invoked as `sh`, dash, ksh93u+ and zsh 5.9.2:
//
//	u=; x=${u:-((a))}; echo "[$x]"      [((a))] in all six
//	u=; y=${u:-$((1+2))}; echo "[$y]"   [3] in all six
//
// so the rule is about where a *command* may begin and not about arithmetic
// being unavailable in an operand. See docs/spec/grammar/parameter-expansion.md.
//
// The assertion is the operand's **text**, because the arithmetic reading is a
// lossy one and text is what it lost: scanArithCommand keeps the expression
// and strips the two opening parens and up to two closing ones, so `((a))`
// came back as the single token `a` and `((#s)a)` as `#s)a` — which is the
// string that reached the matcher as `bad pattern: #s)a` (#1408).
//
// A parse test rather than only a run test: nothing downstream can recover
// characters the lexer did not hand it, so this is the layer the fault lived
// at. The dialect is [syntax.Core] with the operators switched on one at a
// time, so no shell is named here; which shell reads what lives in dialect/.
func TestAnExpansionsOperandIsNotACommandPosition(t *testing.T) {
	d := syntax.Core()
	d.ArithCommand = true
	d.ParamSubstitution = true

	for _, c := range []struct {
		name string
		src  string
		arg  string
		arg2 string
	}{
		// The word operand, which is the unanimous row above.
		{"a word operand", "echo ${u:-((a))}", "((a))", ""},
		// The pattern operands, which is where the same fault was fatal:
		// a leading group is ordinary in the grammar that has groups.
		{"a trim's pattern", "echo ${v#((a))}", "((a))", ""},
		{"a long trim's pattern", "echo ${v##((a))}", "((a))", ""},
		{"a suffix trim's pattern", "echo ${v%((a))}", "((a))", ""},
		{"a flag group inside one", "echo ${v#((#s)a)}", "((#s)a)", ""},
		// Both operands of a replacement, since each is lexed on its own.
		{"a replacement's pattern", "echo ${v/((a))/z}", "((a))", "z"},
		{"a replacement's own text", "echo ${v/z/((a))}", "z", "((a))"},
		// Three parens, so a reading that stripped two closing ones is not
		// the same as one that stripped the balance.
		{"a nested group", "echo ${v#(((a)))}", "(((a)))", ""},
		// Not adjacent, which the `((` rule never reached and which is
		// therefore the control on the control: this always worked.
		{"a spaced pair", "echo ${u:-( (a) )}", "( (a) )", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := paramOf(t, c.src, d)
			if got := argText(e.Arg); got != c.arg {
				t.Errorf("%s: operand = %q, want %q", c.src, got, c.arg)
			}
			if got := argText(e.Arg2); got != c.arg2 {
				t.Errorf("%s: second operand = %q, want %q", c.src, got, c.arg2)
			}
			for _, s := range e.Arg.Spans {
				if s.Kind == syntax.ArithSubst {
					t.Errorf("%s: operand holds an arithmetic span %q — an operand is not a command position",
						c.src, s.Value)
				}
			}
		})
	}
}

// The suspension belongs to the operand and not to arithmetic: `$((` in the
// same position is still an arithmetic substitution, which is the control that
// separates this fix from switching the construct off.
//
// Both operands, and a `((` beside one, because the operand is lexed as a
// whole: a `$((` that stopped being read would leave its text behind as
// literal characters and the row above would still pass.
func TestAnOperandStillTakesAnArithmeticSubstitution(t *testing.T) {
	d := syntax.Core()
	d.ArithCommand = true
	d.ParamSubstitution = true

	for _, c := range []struct {
		src  string
		want string
	}{
		{"echo ${u:-$((1+2))}", "1+2"},
		{"echo ${v#$((1+2))}", "1+2"},
		{"echo ${v/x/$((1+2))}", "1+2"},
		// An arithmetic substitution next to the parentheses that are not
		// one, so the two readings stand in the same operand.
		{"echo ${u:-((a))$((1+2))}", "1+2"},
	} {
		e := paramOf(t, c.src, d)
		w := e.Arg
		if e.Arg2 != nil && len(e.Arg2.Spans) > 0 {
			w = e.Arg2
		}
		var found string
		for _, s := range w.Spans {
			if s.Kind == syntax.ArithSubst {
				found = s.Value
			}
		}
		if found != c.want {
			t.Errorf("%s: arithmetic substitution = %q, want %q", c.src, found, c.want)
		}
	}
}

// An arithmetic command is still one where a command may begin, which is the
// other half of the same control: the operand suspends it, nothing else does.
func TestACommandPositionStillTakesAnArithmeticCommand(t *testing.T) {
	d := syntax.Core()
	d.ArithCommand = true

	for _, src := range []string{
		"((1+2))",
		"x=1; ((x+1)) && echo ran",
		// Inside a substitution, which is a command position of its own and
		// reached through the same operand-lexing path in the parser.
		"echo ${ ((1+2)) ;}",
	} {
		p := syntax.NewParser(src, d)
		f := p.Parse()
		if p.Err() != nil {
			t.Errorf("%s: %v", src, p.Err())
			continue
		}
		if f == nil || len(f.Stmts) == 0 {
			t.Errorf("%s: parsed to nothing", src)
		}
	}
}

// quotedPartsOf is the text of an operand's spans that carry quoting, joined.
//
// It is the other half of argText, and the two are asserted together on
// purpose: the text alone cannot tell a lexer that recorded the quoting from
// one that threw the quotes away, and the quoting alone cannot tell a lexer
// that kept the characters from one that dropped them.
func quotedPartsOf(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	var b strings.Builder
	for _, s := range w.Spans {
		if s.Quoting != syntax.Unquoted {
			b.WriteString(s.Value)
		}
	}
	return b.String()
}

// An expansion's operand is not a command position, so a `#` where a word
// could begin in one is a character of that word rather than a comment.
//
// Measured 2026-09-11 from a script file under `env -i`, on zsh 5.9.2, bash
// 5.3.15, bash 3.2.57 and dash — and unanimous, which is what makes it the
// core's answer rather than a dialect's:
//
//	u=; printf '[%s]' "${u:-a#b}"    [a#b]
//	u=; printf '[%s]' "${u:-#b}"     [#b]
//	v='a #b'; r=${v/a #'b'/Q}        Q in the four with the operator
//
// The last row is the one that matters, and the reason is the *repair* rather
// than the skip. This lexer consumed the comment — everything from the `#` to
// the end of the operand — and Parser.wordFrom put the consumed run back as a
// single raw literal span, so the text reappeared and the quoting inside it
// did not. `${v/(#b)\X/Q}` reached the matcher as the five characters
// `(#b)\X` with a live backslash, against a pattern whose text is `(#b)X`, and
// every glob flag group in a substitution stopped matching as soon as anything
// behind it was quoted or escaped (#2074).
//
// Which is why both halves are asserted. The text was already right for the
// rows with no quoting in them, so a text-only test passes against the bug.
func TestAnExpansionsOperandHasNoComment(t *testing.T) {
	d := syntax.Core()
	d.ParamSubstitution = true

	for _, c := range []struct {
		name   string
		src    string
		arg    string
		quoted string
		arg2   string
	}{
		// A `#` mid-word never opened a comment and is the control.
		{"a hash inside a word", "echo ${u:-a#b}", "a#b", "", ""},
		// A `#` where a word could begin, which is where one would.
		{"a hash opening the operand", "echo ${u:-#b}", "#b", "", ""},
		{"a hash after a blank", "echo ${u:-a #b}", "a #b", "", ""},
		// The quoting behind it, which is what the repair lost.
		{"single quotes behind a hash", "echo ${u:-a #'b'}", "a #b", "b", ""},
		{"double quotes behind a hash", `echo ${u:-a #"b"}`, "a #b", "b", ""},
		{"a backslash behind a hash", `echo ${u:-a #\b}`, "a #b", "b", ""},
		// A pattern operand, where the same loss changes what matches.
		{"a trim's pattern", "echo ${v#a #'b'}", "a #b", "b", ""},
		{"a replacement's pattern", "echo ${v/a #'b'/Q}", "a #b", "b", "Q"},
		{"a replacement's own text", "echo ${v/x/a #'b'}", "x", "", "a #b"},
		// A flag group in front of an escape, which is the shape #2074 was
		// filed as: the `(` is not a group the core reads, so the `#` behind
		// it stands where a word could begin.
		{"a flag group and an escape", `echo ${v/(#b)\X/Q}`, "(#b)X", "X", "Q"},
		{"a flag group and a quote", `echo ${v/(#b)(*)'X'/Q}`, "(#b)(*)X", "X", "Q"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := paramOf(t, c.src, d)
			if got := argText(e.Arg); got != c.arg {
				t.Errorf("%s: operand = %q, want %q", c.src, got, c.arg)
			}
			if got := quotedPartsOf(e.Arg); got != c.quoted {
				t.Errorf("%s: quoted parts of the operand = %q, want %q", c.src, got, c.quoted)
			}
			if got := argText(e.Arg2); got != c.arg2 {
				t.Errorf("%s: second operand = %q, want %q", c.src, got, c.arg2)
			}
		})
	}
}

// The suspension belongs to the operand and not to the comment: a `$( )` in an
// operand is a command line again, and a `#` there opens a comment as it does
// anywhere else.
//
// The assertion is the substitution's **body**, and a `)` inside the comment
// is what makes it discriminating: the raw scan that finds the closing paren
// counts parentheses, so a comment it did not honor would close the
// substitution early and hand back a shorter body. That is the shape a
// coarser fix takes — switching the sub-lexer's CommentMode to
// CommentsOrdinaryText suspends the comment for this scan too, and this row is
// the one that says so.
//
// The panel is unanimous the other way here, which is why it is not folded
// into the rule above: `u=; printf '[%s]' "${u:-$(echo hi # there)}"` is a
// parse failure on zsh 5.9.2, bash 5.3.15, bash 3.2.57 and dash alike, the
// `#` having swallowed the closing paren.
func TestASubstitutionInsideAnOperandStillHasComments(t *testing.T) {
	d := syntax.Core()
	d.ParamSubstitution = true

	const src = "echo ${u:-$(echo a\n# b)\necho c)}"
	e := paramOf(t, src, d)
	var body string
	for _, s := range e.Arg.Spans {
		if s.Kind == syntax.CommandSubst {
			body = s.Value
		}
	}
	if want := "echo a\n# b)\necho c"; body != want {
		t.Errorf("the substitution's body = %q, want %q", body, want)
	}
}
