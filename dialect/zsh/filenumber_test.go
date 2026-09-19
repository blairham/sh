// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// The operand of `<&` is a file number here and an ordinary word in the other
// four columns, which is why a shipped completion function's line parsed too
// easily: `tcp_point` redirects from a descriptor held in a parameter, and
// real `zsh -n` refuses it.
//
// Measured 2026-09-18 on zsh 5.9.2 under `-f`, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with stdin on /dev/null. The sentence quotes
// nothing, the command is given up at 1, and the script runs on (#3144).
func TestAnInputDuplicatesOperandIsAFileNumberHere(t *testing.T) {
	d := zsh.Dialect()
	if !d.InputDuplicateOperandIsAFileNumber {
		t.Error("InputDuplicateOperandIsAFileNumber is false, want true")
	}
	for _, src := range []string{
		"cat <&$v", `cat <&"$v"`, "cat <&${v}", "cat <&$(echo 5)", "cat <&x",
		"cat <&5x", "exec {fd}<&$v",
	} {
		_, err := syntax.Parse(src, d.On(syntax.RouteFromScriptFile))
		if err == nil {
			t.Errorf("%q parsed; this shell refuses it", src)
			continue
		}
		if got, want := zsh.Diagnostics().ForScript().ParseFailure(err), "file number expected"; got != want {
			t.Errorf("%q: got %q, want %q", src, got, want)
		}
	}
	// A file number, the `-` that closes the descriptor, the coprocess `p`,
	// and a quoted number — which is still the digits.
	for _, src := range []string{"cat <&5", "cat <&55", "cat <&-", "cat <&p", `cat <&"5"`, "cat 3<&4"} {
		if _, err := syntax.Parse(src, d.On(syntax.RouteFromScriptFile)); err != nil {
			t.Errorf("parse %q: %v", src, err)
		}
	}
	// And `>&` is not the question: that operator also spells "send both
	// streams to this file", so a word there is a path.
	for _, src := range []string{"cat >&$v", "cat >&x", "cat 2>&$v"} {
		if _, err := syntax.Parse(src, d.On(syntax.RouteFromScriptFile)); err != nil {
			t.Errorf("parse %q: %v", src, err)
		}
	}
}

// The line and not the file: the line before it runs, the line after it runs,
// and the refusal is what the driver gives the line up for.
func TestARefusedFileNumberGivesUpItsLineHere(t *testing.T) {
	d := zsh.Dialect()
	p := syntax.NewParser("echo one\ncat <&$v\necho two\n", d.On(syntax.RouteFromScriptFile))
	var refused, ran int
	for {
		line, ok := p.NextLine()
		if !ok {
			break
		}
		if line.Refused != nil {
			refused++
			if got := zsh.Diagnostics().ForScript().ParseFailure(line.Refused); !strings.Contains(got, "file number expected") {
				t.Errorf("the refused line says %q", got)
			}
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
