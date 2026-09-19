// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// withContainedHeredocs is the grammar of a shell that requires a
// here-document opened inside a substitution to have its body there too.
//
// HeredocEndsAtClosingParen beside it because the two travel together in the
// one column that has either: a body *is* read from between the parentheses
// there, which is what makes "the body is not inside them" a state worth
// refusing rather than the ordinary reading.
func withContainedHeredocs() Dialect {
	d := Core()
	d.HeredocEndsAtClosingParen = true
	d.HeredocBodyMustBeInsideTheSubstitution = true
	return d
}

// A here-document opened inside a one-line `$( )` has nowhere to put its body:
// the substitution's text ends on that line, so the body would have to come
// from the lines after the *enclosing* command.
//
//	echo $(cat <<EOF)
//	body
//	EOF
//
// With the flag off the substitution is empty and those lines are program
// text, which is the reading four of the six columns have. With it on the
// construct is refused while the line is read, which is the fifth column's.
func TestAHereDocumentWhoseBodyIsOutsideItsSubstitutionIsADialectQuestion(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src, token string
		line             int
	}{
		{
			name:  "the body would begin after the enclosing command",
			src:   "echo $(cat <<EOF)\nbody\nEOF\n",
			token: "<<EOF", line: 1,
		},
		{
			// The operator's own line, not the one the substitution opened
			// on: the two are apart here and the refusal names the second.
			name:  "the operator is on a later line than the opener",
			src:   "echo $(echo a\ncat <<EOF)\nbody\nEOF\n",
			token: "<<EOF", line: 2,
		},
		{
			// What the sentence quotes is the operator with the delimiter's
			// quoting off: the `-` is not written, a descriptor in front of
			// the operator is not written, and the quotes come off the word.
			name:  "a stripping operator quotes the plain form",
			src:   "echo $(cat <<-EOF)\nbody\nEOF\n",
			token: "<<EOF", line: 1,
		},
		{
			name:  "a quoted delimiter quotes the plain form",
			src:   "echo $(cat <<\"EOF\")\nbody\nEOF\n",
			token: "<<EOF", line: 1,
		},
		{
			name:  "an escaped delimiter quotes the plain form",
			src:   "echo $(cat <<\\EOF)\nbody\nEOF\n",
			token: "<<EOF", line: 1,
		},
		{
			name:  "a descriptor in front of the operator is left out",
			src:   "echo $(cat 3<<EOF)\nbody\nEOF\n",
			token: "<<EOF", line: 1,
		},
		{
			// A delimiter with blanks in it keeps them, which is what says
			// the token is the delimiter's text rather than a word.
			name:  "a delimiter with blanks keeps them",
			src:   "echo $(cat <<'E O F')\nbody\nE O F\n",
			token: "<<E O F", line: 1,
		},
		{
			name:  "a command with something after it in the substitution",
			src:   "echo $(cat <<EOF; echo x)\nbody\nEOF\n",
			token: "<<EOF", line: 1,
		},
		{
			// The inner construct is what is refused, and the refusal travels
			// out through the outer read that failed with it rather than
			// waiting for the value to be read again.
			name:  "a substitution written inside another",
			src:   "echo $(echo $(cat <<EOF))\nbody\nEOF\n",
			token: "<<EOF", line: 1,
		},
		// A `$( )` written inside a **parameter-expansion word** or inside an
		// **arithmetic expansion** reaches the lexer only through a scan that
		// counts parentheses, so none of these arrived at the point the text
		// was read: the unbraced words ran the body as commands at 0 and the
		// quoted and arithmetic ones raised the refusal from the re-parse the
		// value gets at expansion time, located as a line the shell was
		// running (#3719). The rows are here rather than in a file of their
		// own because the answer is the same answer — same kind, same token,
		// same line — which is the claim.
		{
			name:  "inside an unbraced parameter-expansion word",
			src:   "echo ${x-$(cat <<EOF)}\nbody\nEOF\n",
			token: "<<EOF", line: 1,
		},
		{
			// The colon form, which used to answer differently from the
			// bare one for no reason either shell has.
			name:  "inside a colon-dash word",
			src:   "echo ${x:-$(cat <<EOF)}\nbody\nEOF\n",
			token: "<<EOF", line: 1,
		},
		{
			// The same word inside double quotes, which is a second route to
			// one word: the two used to disagree with *each other*, which is
			// a fact about this lexer rather than about any shell.
			name:  "inside a quoted parameter-expansion word",
			src:   "echo \"${x-$(cat <<EOF)}\"\nbody\nEOF\n",
			token: "<<EOF", line: 1,
		},
		{
			name:  "inside a trimming word",
			src:   "echo ${x#$(cat <<EOF)}\nbody\nEOF\n",
			token: "<<EOF", line: 1,
		},
		{
			name:  "inside an arithmetic expansion",
			src:   "echo $((1+$(cat <<EOF)))\nbody\nEOF\n",
			token: "<<EOF", line: 1,
		},
		{
			// The arithmetic expansion whose whole operand is the
			// substitution, which is the shape a script writes.
			name:  "the whole of an arithmetic operand",
			src:   "echo $(( $(cat <<EOF) ))\nbody\nEOF\n",
			token: "<<EOF", line: 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var se *Error
			_, err := Parse(c.src, withContainedHeredocs())
			if err == nil || !errors.As(err, &se) {
				t.Fatalf("%q = %v, want a refusal", c.src, err)
			}
			if se.Kind != ErrHeredocOutsideSubstitution {
				t.Errorf("%q kind = %v, want ErrHeredocOutsideSubstitution", c.src, se.Kind)
			}
			if se.Token != c.token {
				t.Errorf("%q token = %q, want %q", c.src, se.Token, c.token)
			}
			if got := int(se.Pos.Line); got != c.line {
				t.Errorf("%q line = %d, want %d", c.src, got, c.line)
			}
			// And the control: with the flag off the same text parses, so
			// this is the flag's answer rather than something the grammar
			// refuses either way.
			off := withContainedHeredocs()
			off.HeredocBodyMustBeInsideTheSubstitution = false
			if _, err := Parse(c.src, off); err != nil {
				t.Errorf("%q with the flag off = %v, want it read", c.src, err)
			}
		})
	}
}

// And what the flag does not reach, which is the half a rule keyed on "a
// here-document inside parentheses" would get wrong.
func TestTheContainedHeredocRuleIsAboutTheSubstitutionsOwnText(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src string }{
		{
			// The body is inside the substitution, which is the whole point:
			// the closer is on a line of its own past the delimiter.
			"a body inside the parentheses",
			"echo $(cat <<EOF\nbody\nEOF\n)\n",
		},
		{
			// And with the closer on the delimiter's own line, which is what
			// HeredocEndsAtClosingParen is for — the body still ends inside.
			"a delimiter carrying the closer",
			"echo $(cat <<EOF\nbody\nEOF)\n",
		},
		{
			// A here-string is not a here-document: its input is its own
			// word, on the line the operator is on.
			"a here-string",
			"echo $(cat <<<word)\n",
		},
		{
			// The backquoted spelling takes a body from the whole input, a
			// body being unable to hold the mark that would close it. It is
			// not refused in the column this flag is from.
			"the backquoted spelling",
			"echo `cat <<EOF`\nbody\nEOF\n",
		},
		{
			// And a process substitution is a different construct: measured
			// as accepted in the one column that refuses the two spellings
			// above it.
			"a process substitution",
			"cat <(cat <<EOF)\nbody\nEOF\n",
		},
		{
			"a substitution with no here-document in it at all",
			"echo $(echo hi)\necho ok\n",
		},
		// The contained shapes of the two routes the refusal now reaches,
		// which are the controls that say the new rows are about the body
		// being *outside* rather than about a `$( )` in a word at all. Both
		// run to completion in the column this flag is from.
		{
			"a contained body inside a parameter-expansion word",
			"echo ${x-$(cat <<EOF\nbody\nEOF\n)}\necho ok\n",
		},
		{
			"a contained body inside an arithmetic expansion",
			"echo $(( 1 + $(cat <<EOF\n2\nEOF\n) ))\necho ok\n",
		},
		{
			"an ordinary word and an ordinary expansion",
			"echo ${x-$(echo two)} $((1+$(echo 2)))\necho ok\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse(c.src, withContainedHeredocs()); err != nil {
				t.Errorf("%q = %v, want it read", c.src, err)
			}
		})
	}
}
