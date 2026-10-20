// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestADefinitionsRefusalFollowsTheReadersUnit — a refusal raised by a
// function **definition** is located where this shell's own counter stands, and
// that is three things rather than the definition's end.
//
// A definition is not a simple command, so the shell reporting one of its
// refusals has no command line to point at and points at its counter instead.
// Measured 2026-09-23 on bash 5.3.20 over 66 shapes and all three refusals a
// definition raises — a special builtin's name in POSIX mode, a redefinition of
// a frozen name, and a name that is not a name — which answer identically.
//
// Every row below is one the previous reading, "the definition's last line",
// cannot explain. See interp.Runner.functionDefinitionRefusalLine (#4174).
func TestADefinitionsRefusalFollowsTheReadersUnit(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// Term 2, the unit the reader took: a `;` puts two definitions in one
		// unit, so the first is reported at the *second* one's last line.
		{
			"a second definition on the same line carries the number",
			"set -o posix\nbreak() { :; }; continue() {\n:\n}\necho after\n",
			"sh: line 4: `break': is a special builtin\n",
		},
		{
			"and a continuation does too",
			"set -o posix\nbreak() { :; }; echo a \\\n b \\\n c\necho after\n",
			"sh: line 4: `break': is a special builtin\n",
		},
		// Which the enclosing statement's own shape does not decide: an `if`,
		// a group and a subshell each report their statement's last line.
		{
			"inside an if, the statement's last line",
			"set -o posix\nif true; then\nbreak() { :; }\nfi\necho after\n",
			"sh: line 4: `break': is a special builtin\n",
		},
		{
			"inside a group, the statement's last line",
			"set -o posix\n{\necho pre\nbreak() { :; }\n}\necho after\n",
			"pre\nsh: line 5: `break': is a special builtin\n",
		},
		// Term 1: a `for`, `case` or `select` the frame is running takes the
		// number to its own first line, innermost first, and a `while` is the
		// control that says it is those three and not every compound.
		{
			"inside a for, the loop's own line",
			"set -o posix\nfor i in 1; do\necho pre\nbreak() { :; }\ndone\necho after\n",
			"pre\nsh: line 2: `break': is a special builtin\n",
		},
		{
			"the innermost for of two",
			"set -o posix\nfor i in 1; do\nfor j in 1; do\nbreak() { :; }\ndone\ndone\n",
			"sh: line 3: `break': is a special builtin\n",
		},
		{
			"inside a case, the case's own line",
			"set -o posix\ncase x in\nx)\nbreak() { :; };;\nesac\n",
			"sh: line 2: `break': is a special builtin\n",
		},
		{
			"a while is the control and takes the statement",
			"set -o posix\nwhile [ -z \"$d\" ]; do\nd=1\nbreak() { :; }\ndone\n",
			"sh: line 5: `break': is a special builtin\n",
		},
		{
			"and a loop that has finished takes it back",
			"set -o posix\nfor i in 1; do\n:\ndone\nbreak() { :; }\n",
			"sh: line 5: `break': is a special builtin\n",
		},
		// A subshell is a unit of its own, which is also why the loop register
		// does not cross into one.
		{
			"inside parentheses in a loop, the closing one",
			"set -o posix\nfor i in 1; do\n(\nbreak() { :; }\n)\ndone\n",
			"sh: line 5: `break': is a special builtin\n",
		},
		// Term 3: inside a call there is no unit being read, and the number is
		// the definition's own *first* line — which is where the previous
		// reading is furthest out.
		{
			"inside a call, the definition's first line",
			"set -o posix\nf() {\nbreak() {\n:\n}\n}\nf\necho after\n",
			"sh: line 3: `break': is a special builtin\n",
		},
		{
			"and not the command before it",
			"set -o posix\nf() {\necho pre\nbreak() { :; }\n}\nf\n",
			"pre\nsh: line 4: `break': is a special builtin\n",
		},
		{
			"a for inside the body still wins",
			"set -o posix\nf() {\nfor i in 1; do\nbreak() { :; }\ndone\n}\nf\n",
			"sh: line 3: `break': is a special builtin\n",
		},
		// And the loop the *caller* is in does not reach the callee.
		{
			"a loop around the call is not the callee's",
			"set -o posix\nf() {\nbreak() { :; }\n}\nfor i in 1; do\nf\ndone\n",
			"sh: line 3: `break': is a special builtin\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if out != tc.want {
				t.Errorf("got\n%s\nwant\n%s", out, tc.want)
			}
		})
	}
}
