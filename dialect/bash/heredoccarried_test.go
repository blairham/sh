// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A here-document a one-line substitution opened and could not feed, whose
// body comes from the lines after the enclosing command.
//
// Measured 2026-09-20 against bash 5.3.20, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null device, over
//
//	echo one
//	echo $(cat <<EOF)
//	body
//	EOF
//	echo after
//
// and the two other spellings of the same file:
//
//	spelling     bash 5.3.20                   bash 3.2.57
//	$( )         warned, then `body`           quiet reading
//	<( )         warned, then `body`           quiet reading
//	backquoted   a different warning, quiet    quiet reading
//
// where the *quiet reading* — zsh 5.9.2, dash 0.5.12 and BusyBox ash take it
// for every spelling — is an empty substitution with `body` and `EOF` then
// run as commands.
//
// So it is one spelling pair, one lineage and one build of it. See
// syntax.Dialect.HeredocBodyFromAfterTheCommand (#3711).
func TestAHeredocOpenedInAOneLineSubstitutionIsFedFromAfterTheCommand(t *testing.T) {
	for _, c := range []struct {
		name, src, out, errs string
		status               int
	}{
		{
			// The issue's own case.
			name:   "the newer spelling yields the body",
			src:    "echo one\necho $(cat <<EOF)\nbody\nEOF\necho after\n",
			out:    "one\nbody\nafter\n",
			errs:   "warning: command substitution: 1 unterminated here-document",
			status: 0,
		},
		{
			// The process substitution is the same capability and is what
			// says it is not the command substitution's alone: ksh93 reads
			// this one and refuses the other.
			name:   "the process substitution yields it too",
			src:    "echo one\ncat <(cat <<EOF)\nbody\nEOF\necho after\n",
			out:    "one\nbody\nafter\n",
			errs:   "warning: command substitution: 1 unterminated here-document",
			status: 0,
		},
		{
			// Two of them, in the order the text opened them, and the noun
			// agrees with the count. This is the row that says the warning
			// carries a number rather than being one sentence.
			name:   "two documents, in order, counted",
			src:    "echo one\necho $(cat <<A; cat <<B)\nbody-one\nA\nbody-two\nB\necho after\n",
			out:    "one\nbody-one body-two\nafter\n",
			errs:   "warning: command substitution: 2 unterminated here-documents",
			status: 0,
		},
		{
			// The control that keeps the capability off the older spelling.
			// bash writes a warning here as well and it is the *ordinary*
			// one, about a document delimited by end of file — which is
			// already this shell's, and is why a capability reached from the
			// backquote scanner would have been wrong.
			name:   "the older spelling takes the quiet reading",
			src:    "echo one\necho `cat <<EOF`\nbody\nEOF\necho after\n",
			out:    "one\n\nafter\n",
			errs:   "command not found",
			status: 0,
		},
		{
			// The control that says this is about a document the text could
			// not feed. A well-formed one inside the parentheses is read
			// from between them exactly as before, with nothing carried and
			// nothing said.
			name:   "a document that closes inside the parentheses is untouched",
			src:    "v=$(cat <<EOF\ninside\nEOF\n)\necho \"[$v]\"\n",
			out:    "[inside]\n",
			status: 0,
		},
		{
			// And the control that says the lines really are consumed as a
			// body rather than merely produced twice: `EOF` is not run.
			name:   "the body's lines are not run as commands",
			src:    "echo $(cat <<EOF)\nnotacommand\nEOF\n",
			out:    "notacommand\n",
			errs:   "warning: command substitution: 1 unterminated here-document",
			status: 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, status := runScript(t, scriptFile(t, c.src))
			if out != c.out || status != c.status {
				t.Errorf("ran %q: out %q status %d, want %q and %d (errs %q)",
					c.src, out, status, c.out, c.status, errs)
			}
			if c.errs == "" && errs != "" {
				t.Errorf("ran %q: said %q, want silence", c.src, errs)
			}
			if c.errs != "" && !strings.Contains(errs, c.errs) {
				t.Errorf("ran %q: said %q, want it to carry %q", c.src, errs, c.errs)
			}
		})
	}
}
