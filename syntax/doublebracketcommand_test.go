// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"reflect"
	"testing"
)

// The grammar half of [Dialect.DoubleBracketIsACommand]: a `[[` that is the
// name of a command, whose `&&` and `||` are its operands until a `]]` word.

func doubleBracketCommand() Dialect {
	d := POSIX()
	d.DoubleBracketIsACommand = true
	return d
}

// firstCommandArgs is the argument words of src's first statement when that
// statement is one simple command and nothing else, and nil when the statement
// is a list, a pipeline of several commands, or a compound command — which is
// what a `&&` read as the shell's operator makes of it.
func firstCommandArgs(t *testing.T, d Dialect, src string) []string {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	p, ok := f.Stmts[0].Expr.(*Pipeline)
	if !ok || len(p.Cmds) != 1 {
		return nil
	}
	c, ok := p.Cmds[0].(*SimpleCmd)
	if !ok {
		return nil
	}
	var words []string
	for _, w := range c.Args {
		words = append(words, w.Literal())
	}
	return words
}

func TestAConnectiveAfterACommandBracketIsAnOperand(t *testing.T) {
	t.Parallel()
	d := doubleBracketCommand()
	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{"the conditional's own shape", `[[ -n x && -z y ]]`, []string{"[[", "-n", "x", "&&", "-z", "y", "]]"}},
		{"the other connective", `[[ a || b ]]`, []string{"[[", "a", "||", "b", "]]"}},
		// Not command position: the word opens the reading wherever it is.
		{"an argument of another command", `echo [[ a && b ]]`, []string{"echo", "[[", "a", "&&", "b", "]]"}},
		// The operator is still lexed as one, so no blanks are needed.
		{"written without blanks", `echo [[ a&&b ]]`, []string{"echo", "[[", "a", "&&", "b", "]]"}},
		{"with no closing word", `echo [[ a && b`, []string{"echo", "[[", "a", "&&", "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstCommandArgs(t, d, tc.src); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s: args %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

func TestOnlyAnUnquotedBracketWordOpensTheReading(t *testing.T) {
	t.Parallel()
	d := doubleBracketCommand()
	for _, src := range []string{
		`echo "[[" a && echo b`,
		`echo '[[' a && echo b`,
		`echo \[[ a && echo b`,
		`echo x[[ a && echo b`,
		`echo [[x a && echo b`,
		// And a `]]` word closes it, so the list operator after it is one.
		`echo [[ a ]] && echo b`,
		`echo [[ a ]] b && echo c`,
	} {
		if got := firstCommandArgs(t, d, src); got != nil {
			t.Errorf("%s: read as one command %q, want the && to end it", src, got)
		}
	}
	// A `]]x` closes nothing, so the `&&` after it is still an operand.
	src := `echo [[ a ]]x && b`
	want := []string{"echo", "[[", "a", "]]x", "&&", "b"}
	if got := firstCommandArgs(t, d, src); !reflect.DeepEqual(got, want) {
		t.Errorf("%s: args %q, want %q", src, got, want)
	}
}

func TestTheOtherOperatorsKeepTheirMeaningAfterACommandBracket(t *testing.T) {
	t.Parallel()
	d := doubleBracketCommand()
	// A pipe, a separator and a newline each still end the command.
	for _, src := range []string{
		"[[ a | cat ]]",
		"[[ a; echo ]]",
		"[[ a &&\necho b ]]",
	} {
		f, err := Parse(src, d)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if p, ok := f.Stmts[0].Expr.(*Pipeline); ok && len(p.Cmds) == 1 && len(f.Stmts) == 1 {
			t.Errorf("%q: read as one command", src)
		}
	}
	// A redirection is a redirection.
	f, err := Parse("[[ a < b ]]", d)
	if err != nil {
		t.Fatal(err)
	}
	if c := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*SimpleCmd); len(c.Redirs) != 1 {
		t.Errorf("[[ a < b ]]: %d redirections, want 1", len(c.Redirs))
	}
	// And a parenthesis after a word is the refusal it always was.
	refuses(t, d, "[[ ( -n x ) ]]")
	// A for loop's word list is not a simple command's.
	refuses(t, d, "for w in [[ a && b ]]; do :; done")
}

func TestACommandBracketPrintsBackAsWritten(t *testing.T) {
	t.Parallel()
	d := doubleBracketCommand()
	src := `[[ -n x && -z y || a ]]`
	f, err := Parse(src, d)
	if err != nil {
		t.Fatal(err)
	}
	printed := Print(f)
	if got, want := firstCommandArgs(t, d, printed), firstCommandArgs(t, d, src); !reflect.DeepEqual(got, want) {
		t.Errorf("reprint %q reads as %q, want %q", printed, got, want)
	}
}

func TestWithoutTheFlagTheConnectiveEndsTheCommand(t *testing.T) {
	t.Parallel()
	// The control: the same words under a grammar that has neither reading
	// of `[[`, where the `&&` is the list operator.
	if got := firstCommandArgs(t, POSIX(), `echo [[ a && b ]]`); got != nil {
		t.Errorf("read as one command %q without the flag", got)
	}
}
