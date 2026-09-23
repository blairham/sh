// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runBashSplitFatal is runBashSplit for a script that is *meant* to fail: a
// body that will not parse ends the shell in this dialect, and a failed
// redirection leaves a status behind. Both are the answer here rather than a
// harness fault, so the run's own error is returned rather than failing the
// test.
func runBashSplitFatal(t *testing.T, src string) (out, errs string) {
	t.Helper()
	f, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &diag,
		Dir: t.TempDir(), Name: "bash", Dialect: presetDialect(),
	}
	bash.Apply(r)
	_, _ = r.Run(context.Background(), f)
	return o.String(), e.String()
}

// TestASubstitutionBodyIsNumberedFromWhereTheShellWasReading — a body's first
// command is numbered at the line the command that holds the substitution
// reports itself at, and the body's later lines count up from there.
//
// Measured 2026-09-23 against bash 5.3.15 in the pinned debian:sid-slim and
// bash 5.3.20 on macOS, which agree on every row. `echo one` is line one in
// each source so the numbers below are the reference's own, and the two
// spellings with a closer of their own are asked the same questions — see
// Diagnostics.SubstitutionBodyIsNumberedFromWhereTheShellWasReading.
//
// The rows were all wrong before #4155, and wrong in both directions: the axis
// this replaces was measured over five shapes whose first command sat directly
// under the opener, where the opener's line and the answer coincide.
func TestASubstitutionBodyIsNumberedFromWhereTheShellWasReading(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// An assignment's first word ends at the closing delimiter, so that
		// is the line the body's first command takes.
		{"the closer's line", "echo one\nv=$(\necho \"L=$LINENO\"\n)\necho \"[$v]\"\n", "one\n[L=4]\n"},
		{"however many blank lines are in front", "echo one\nv=$(\n\n\necho \"L=$LINENO\"\n)\necho \"[$v]\"\n", "one\n[L=6]\n"},
		{"and the lines below it count up", "echo one\nv=$(\necho x\necho \"L=$LINENO\"\n)\necho \"[$v]\"\n", "one\n[x\nL=6]\n"},
		{"trailing blank lines do not move it", "echo one\nv=$(\necho \"L=$LINENO\"\n\n\n)\necho \"[$v]\"\n", "one\n[L=6]\n"},
		{"a comment takes no line of its own", "echo one\nv=$(\n# c\necho \"L=$LINENO\"\n)\necho \"[$v]\"\n", "one\n[L=5]\n"},
		// The controls: with the command and the closer on one line every
		// column in the panel agrees, so nothing here is a constant offset.
		{"text after the opener", "echo one\nv=$(echo \"L=$LINENO\"\n)\necho \"[$v]\"\n", "one\n[L=3]\n"},
		{"the closer on the command's line", "echo one\nv=$(\necho \"L=$LINENO\")\necho \"[$v]\"\n", "one\n[L=3]\n"},
		// A command's first word is its *name* when the substitution is in a
		// later word, so the body is numbered from the command's own line and
		// the closer is nowhere in it.
		{"an argument is numbered from the command", "echo one\necho \"[$(\necho \"L=$LINENO\"\n)]\"\n", "one\n[L=2]\n"},
		{"and its blank lines are still skipped", "echo one\necho \"[$(\necho \"L=$LINENO\"\n\n\n)]\"\n", "one\n[L=2]\n"},
		{"and its later lines count up", "echo one\necho \"[$(\necho a\necho \"L=$LINENO\"\n)]\"\n", "one\n[a\nL=3]\n"},
		// The current-shell spelling is the same question with a different
		// closer.
		{"the current-shell spelling", "echo one\nv=${\necho \"L=$LINENO\"\n}\necho \"[$v]\"\n", "one\n[L=4]\n"},
		{"quoted, and still the closer", "echo one\nv=\"${\necho \"L=$LINENO\"\n}\"\necho \"[$v]\"\n", "one\n[L=4]\n"},
		// The older spelling counts the newlines the newer one skips, so its
		// anchor is the backquote's line and its body starts one below.
		{"backquoted, on one line", "echo one\nv=`echo \"L=$LINENO\"`\necho \"[$v]\"\n", "one\n[L=2]\n"},
		{"backquoted, opener ends the line", "echo one\nv=`\necho \"L=$LINENO\"\n`\necho \"[$v]\"\n", "one\n[L=5]\n"},
		{"backquoted, blank line in front", "echo one\nv=`\n\necho \"L=$LINENO\"\n`\necho \"[$v]\"\n", "one\n[L=7]\n"},
		{"backquoted, closer on the command", "echo one\nv=`\necho \"L=$LINENO\"`\necho \"[$v]\"\n", "one\n[L=4]\n"},
		{"backquoted in an argument", "echo one\necho \"[`\necho \"L=$LINENO\"\n`]\"\n", "one\n[L=3]\n"},
		// A body inside a body is numbered from the statement that holds it,
		// as that statement is numbered — which is its own line in there,
		// because the counter running a re-parsed body does not advance over a
		// nested substitution's newlines.
		{"a nested body takes its statement's line", "echo one\nv=$(\nw=$(\necho \"I=$LINENO\"\n)\necho \"[$w]\"\n)\necho \"[$v]\"\n", "one\n[[I=7]]\n"},
		{"and the statement below it counts up", "echo one\nv=$(\necho \"O=$LINENO\"\nw=$(\necho \"I=$LINENO\"\n)\necho \"[$w]\"\n)\necho \"[$v]\"\n", "one\n[O=8\n[I=9]]\n"},
		{"an argument outside puts both on its line", "echo one\necho \"[$(\nw=$(\necho \"I=$LINENO\"\n)\necho \"[$w]\"\n)]\"\n", "one\n[[I=2]]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs := runBashSplit(t, tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if errs != "" {
				t.Errorf("stderr = %q, want nothing", errs)
			}
		})
	}
}

// TestASubstitutionBodysRefusalIsWhereItsTextIs — a body that will not parse is
// reported at the line its text is on, which is not the line its commands run
// at.
//
// The pair is the point: the run is numbered from where the shell was reading
// and the refusal is not, because the refusal comes out of the scan that has
// not reached the closing delimiter yet. One offset for both is how this came
// to be one wrong answer twice — see Runner.readSubstBody.
//
// Measured 2026-09-23 in the pinned image over the four shapes below.
func TestASubstitutionBodysRefusalIsWhereItsTextIs(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"under the opener", "echo one\nv=$(\nif; then :; fi\n)\necho \"[$v]\"\n", "line 3:"},
		{"a blank line in front", "echo one\nv=$(\n\nif; then :; fi\n)\necho \"[$v]\"\n", "line 4:"},
		{"the closer far below", "echo one\nv=$(\nif; then :; fi\n\n)\necho \"[$v]\"\n", "line 3:"},
		{"the current-shell spelling", "echo one\nv=${\nif; then :; fi\n}\necho \"[$v]\"\n", "line 3:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := runBashSplitFatal(t, tc.src)
			if !strings.Contains(errs, tc.want) {
				t.Errorf("stderr = %q, want it to name %q", errs, tc.want)
			}
		})
	}
}

// TestACommandIsLocatedWhereItsFirstWordEnds — a command whose first word spans
// lines reports itself at the line that word ends on, not at the line it began
// on.
//
// bash and dash; zsh and ksh93 name the line the command began on. Measured
// 2026-09-23 over the five shapes below, and the last two are the controls: with
// the multi-line word in a *later* position, every column names the command's
// own line however far below it runs on. See Runner.commandLine.
func TestACommandIsLocatedWhereItsFirstWordEnds(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a multi-line substitution", "echo one\nv=$(\n:\n) > /nonexistent42/f\n", "line 4:"},
		{"a multi-line quoted string", "echo one\nv=\"a\nb\" > /nonexistent42/f\n", "line 3:"},
		{"a backslash-newline", "echo one\nv=a\\\nb > /nonexistent42/f\n", "line 3:"},
		{"the substitution in a later word", "echo one\necho x \"$(\n:\n)\" > /nonexistent42/f\n", "line 2:"},
		{"a name that is a substitution", "echo one\n\"$(\necho nosuchcmd42\n)\"\n", "line 4:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := runBashSplitFatal(t, tc.src)
			if !strings.Contains(errs, tc.want) {
				t.Errorf("stderr = %q, want it to name %q", errs, tc.want)
			}
		})
	}
	// And the same number anchors an ERR trap, which is what says this is the
	// command's location and not one message's wording.
	out, _ := runBashSplitFatal(t, "trap 'echo \"E=$LINENO\"' ERR\nv=$(\nfalse\n)\n")
	if want := "E=4\n"; out != want {
		t.Errorf("ERR trap: got %q, want %q", out, want)
	}
}
