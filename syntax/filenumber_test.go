// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// The operand of `<&` is a file number in one column and an ordinary word in
// the rest. These name the flag and never a shell.

// A run of digits, a `-` or the coprocess `p`, and nothing else. Measured
// 2026-09-18 on zsh 5.9.2 under `-f`, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` (#3144).
func TestWhetherAnInputDuplicatesOperandMustBeAFileNumber(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		src     string
		refused bool
	}{
		{"cat <&5", false},
		{"cat <&55", false},
		{"cat <&-", false},
		{"cat <&p", false},
		{"cat 3<&4", false},
		// The quoted rows are what say it is the operand's *text* rather
		// than its spelling.
		{`cat <&"5"`, false},
		{`cat <&"$v"`, true},
		{"cat <&$v", true},
		{"cat <&${v}", true},
		{"cat <&$(echo 5)", true},
		{"cat <&`echo 5`", true},
		{"cat <&x", true},
		{"cat <&5x", true},
		{"cat <&{fd}", true},
		// The literal text rather than what the word would come to: a digit
		// the parser can see is enough, and an expansion beside it is not
		// looked into. `$v5` is the *parameter* `v5` — one span with no
		// literal text — which is why it goes the other way, and `5x` is
		// literal text that is not a digit, which refuses whatever stands
		// beside it.
		{"cat <&5$v", false},
		{"cat <&${v}5", false},
		{`cat <&"5""$v"`, false},
		{"cat <&$(echo 5)5", false},
		{"cat <&5$(echo x)", false},
		{"cat <&$v5", true},
		{"cat <&$v$w", true},
		{"cat <&x$v", true},
		{`cat <&"x"$v`, true},
		{"cat <&${v}x", true},
		{`cat <&""$v`, true},
		// And it reaches the operator wherever it stands.
		{"exec {fd}<&$v", true},
		// The control, and the reason this is not "a duplicating
		// redirection's operand": `>&` also spells "send both streams to
		// this file", so a word there is a path.
		{"cat >&$v", false},
		{"cat >&x", false},
		{"cat 2>&$v", false},
		{"cat <<<$v", false},
	} {
		for _, on := range []bool{false, true} {
			d := syntax.Core()
			d.ProcessSubstitution = true
			d.InputDuplicateOperandIsAFileNumber = on
			f, err := syntax.Parse(c.src, d)
			refused := err != nil || (f != nil && f.Refused != nil)
			if want := on && c.refused; refused != want {
				t.Errorf("on=%v %q: refused=%v, want %v (err %v)", on, c.src, refused, want, err)
			}
		}
	}
}

// What the refusal carries: its own kind, and the operand as written, so a
// dialect may word it without matching on the message.
func TestARefusedFileNumberCarriesItsOwnKind(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.InputDuplicateOperandIsAFileNumber = true
	_, err := syntax.Parse("cat <&$v", d)
	var se *syntax.Error
	if err == nil || !errorAs(err, &se) {
		t.Fatalf("err = %v, want a syntax error", err)
	}
	if se.Kind != syntax.ErrFileNumber {
		t.Errorf("Kind = %v, want ErrFileNumber", se.Kind)
	}
	if se.Token != "$v" {
		t.Errorf("Token = %q, want %q", se.Token, "$v")
	}
}

// It gives up the **line** and not the file, which is measured: the line
// before it runs, the line after it runs. NextLine is the route that shows it;
// reading the whole file at once has no next line to go on to and carries the
// refusal into the parser's own error, which is what `-n` reports.
func TestARefusedFileNumberGivesUpItsLine(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.InputDuplicateOperandIsAFileNumber = true
	p := syntax.NewParser("echo one\ncat <&$v\necho two\n", d)
	var refused, ran int
	for {
		line, ok := p.NextLine()
		if !ok {
			break
		}
		if line.Refused != nil {
			refused++
			continue
		}
		ran += len(line.Stmts)
	}
	if err := p.Err(); err != nil {
		t.Fatalf("the file did not read: %v", err)
	}
	if refused != 1 || ran != 2 {
		t.Errorf("refused=%d ran=%d, want 1 and 2", refused, ran)
	}
}

func errorAs(err error, out **syntax.Error) bool {
	for err != nil {
		if se, ok := err.(*syntax.Error); ok {
			*out = se
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
