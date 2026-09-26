// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A here-document **body** that failed is located at the line the whole
// command ended on, for a compound command spelled with a reserved word.
//
// See interp/heredoccommandend.go for the grid this is read off and for the
// controls behind each half of it (#4690, #4712).

// bodyFailureLine runs src with the axis set and returns what it wrote to
// standard error, with a location style that shows the line.
func bodyFailureLine(t *testing.T, src string, atTheEnd bool) string {
	t.Helper()
	var errs strings.Builder
	sem := PosixSemantics()
	// A failing body is the redirection's, which is the reading that keeps
	// the failure on the command rather than ending the run.
	sem.HeredocBodyFailureIsTheRedirections = Yes
	sem.HeredocExpandsInTheCommandsProcess = No
	sem.HeredocBodyOnASubshellExpandsInTheSubshell = No
	dg := Diagnostics{
		Location:   LocationTightLine,
		ArithError: "arithmetic: %[1]s",
		HeredocBodyOnAReservedWordCompoundIsLocatedWhereTheCommandEnds: atTheEnd,
	}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
		Stdout: &strings.Builder{}, Stderr: &errs,
	})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return errs.String()
}

// The command is on line 1, its body on line 2 and its delimiter on line 3.
func bodyOn(command string) string {
	return command + " <<END\n$(( 1/0 ))\nEND\necho done\n"
}

// A compound spelled with a reserved word is located where the command ended;
// every other spelling is located where it began.
//
// The controls are the point of the table: `( … )`, `[[ … ]]` and an
// arithmetic command are all compound commands and all report the line they
// began on, which is why this is keyed on the spelling.
func TestAFailingBodyIsLocatedWhereAReservedWordCompoundEnded(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, command string
		want          string
	}{
		{"a group", "{ :; }", ":3:"},
		{"a while loop", "while read -r x; do :; done", ":3:"},
		{"an until loop", "until :; do :; done", ":3:"},
		{"an if", "if :; then :; fi", ":3:"},
		{"a for loop", "for i in 1; do :; done", ":3:"},
		{"a case", "case x in x) :;; esac", ":3:"},
		// The controls.
		{"a subshell", "( : )", ":1:"},
		{"a condition", "[[ x = x ]]", ":1:"},
		{"an arithmetic command", "(( 1 ))", ":1:"},
		{"a builtin", ":", ":1:"},
		{"an external command", "cat", ":1:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := bodyFailureLine(t, bodyOn(tc.command), true)
			if !strings.Contains(got, tc.want) {
				t.Errorf("said %q, want the line %s", got, tc.want)
			}
			// And with the axis off every one of them is where it began,
			// which is what says the controls are not passing by accident.
			if off := bodyFailureLine(t, bodyOn(tc.command), false); !strings.Contains(off, ":1:") {
				t.Errorf("with the axis off: said %q, want line 1", off)
			}
		})
	}
}

// The number is where the whole command ended and not the delimiter's line,
// which four shapes separate: the command's own text can run on past its
// here-document, and a second here-document closes below the first.
func TestTheLineIsWhereTheWholeCommandEnded(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a pipeline continued below the body",
			"{ :; } <<END |\n$(( 1/0 ))\nEND\ncat\necho done\n",
			":4:",
		},
		{
			"two here-documents",
			"{ :; } <<A <<B\na1\nA\n$(( 2/0 ))\nB\necho done\n",
			":5:",
		},
		{
			"inside an if, which closes below it",
			"if :; then\n{ :; } <<END\n$(( 1/0 ))\nEND\nfi\necho done\n",
			":5:",
		},
		{
			"and further below it",
			"if :; then\n{ :; } <<END\n$(( 1/0 ))\nEND\necho mid\nfi\necho done\n",
			":6:",
		},
		{
			"a loop whose body is above its redirection",
			"for i in 1\ndo :\ndone <<END\n$(( 1/0 ))\nEND\necho done\n",
			":5:",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := bodyFailureLine(t, tc.src, true); !strings.Contains(got, tc.want) {
				t.Errorf("said %q, want the line %s", got, tc.want)
			}
		})
	}
}

// Only the **body** moves. A redirection that could not be opened is
// [Diagnostics.RedirectFailureLine]'s question and is unmoved, and so is a
// command reported after the redirections came out.
func TestOnlyTheBodyTakesTheCommandsEnd(t *testing.T) {
	t.Parallel()
	got := bodyFailureLine(t, "{ :; } < /nonexistent/x\necho done\n", true)
	if !strings.Contains(got, ":1:") {
		t.Errorf("a failed open said %q, want the line the command began on", got)
	}
	body := bodyFailureLine(t, "{ nosuchcommand; } <<'END'\nx\nEND\n", true)
	if !strings.Contains(body, ":1:") {
		t.Errorf("a command inside the body said %q, want its own line 1", body)
	}
}

// A function body is left alone: the reader's position in force inside one is
// the caller's, so the rule does not apply there.
func TestAFunctionBodyIsLeftAlone(t *testing.T) {
	t.Parallel()
	const src = "f() {\n{ :; } <<END\n$(( 1/0 ))\nEND\n}\necho a\nf\n"
	on, off := bodyFailureLine(t, src, true), bodyFailureLine(t, src, false)
	if on != off {
		t.Errorf("with the axis on %q and off %q, want the two the same", on, off)
	}
	if !strings.Contains(on, ":2:") {
		t.Errorf("said %q, want the group's own line 2", on)
	}
}

// And a refusal written **inside** the body moves with it, so that the body's
// own numbering and the command's answer cannot disagree.
//
// A `$( … )` in the body that will not parse is the shape: measured
// 2026-09-26 over a script file, bash 5.3.20 names line 5 for a body on line
// 3 of a `{ f; } <<END` opened on line 2 — one past the delimiter, because
// the refusal counts from where the reader had got to and the body's first
// line is the next one. The same body on a command of its own is at the
// body's own line in both shells, which is the control (#4712).
func TestARefusalInTheBodyMovesWithTheCommandsEnd(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want, off string }{
		{
			"a group",
			"f() { cat; }\n{ f; } <<END\n$(echo hi; for)\nEND\n",
			":5:", ":3:",
		},
		{
			"a loop",
			"while read -r x; do :; done <<END\n$(echo hi; for)\nEND\n",
			":4:", ":2:",
		},
		{
			// The control: no reserved word, so nothing moves.
			"a command of its own",
			"cat <<END\n$(echo hi; for)\nEND\n",
			":2:", ":2:",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := bodyFailureLine(t, tc.src, true); !strings.Contains(got, tc.want) {
				t.Errorf("said %q, want the line %s", got, tc.want)
			}
			if got := bodyFailureLine(t, tc.src, false); !strings.Contains(got, tc.off) {
				t.Errorf("with the axis off: said %q, want the line %s", got, tc.off)
			}
		})
	}
}
