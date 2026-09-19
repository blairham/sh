// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// refuseAsKsh parses src as this dialect and words the failure the way the
// front end does, which is where the sentence lives.
func refuseAsKsh(t *testing.T, src string) string {
	t.Helper()
	_, err := syntax.Parse(src, ksh.Dialect())
	if err == nil {
		t.Fatalf("%q parsed, want a refusal", src)
	}
	return ksh.Diagnostics().ParseDiagnostic("ksh", "-c", err, src)
}

// A here-document opened inside a one-line `$( )` is refused while the line is
// read, and the four other columns leave the substitution empty and run the
// body's lines as commands.
//
// Measured 2026-09-18 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with standard input on the null device, over
//
//	echo one
//	echo $(cat <<EOF)
//	body
//	EOF
//	echo after
//
//	bash 5.3.20   `one`, a warning that a here-document was unterminated,
//	              `body`, `after` — the body is read from the lines after the
//	              enclosing command
//	zsh 5.9.2     `one`, an empty substitution, `body` and `EOF` run as
//	              commands, `after`
//	dash 0.5.12   the same, in its own words
//	BusyBox ash   the same
//	ksh93u+       `one`, then `syntax error at line 2: `<<EOF' here-document
//	              not contained within command substitution`, status 3, and
//	              `after` never reached
//
// `one` running first is what says the refusal arrives **while the line is
// read** rather than before the file is run at all, and this shell has that
// already: it parses incrementally, so what is asserted here is the sentence,
// its line and its token. The end-to-end shape was compared against the real
// shell separately and agrees character for character.
func TestAHereDocumentWhoseBodyIsOutsideItsSubstitutionIsRefused(t *testing.T) {
	got := refuseAsKsh(t, "echo one\necho $(cat <<EOF)\nbody\nEOF\necho after\n")
	want := "ksh: syntax error at line 2: `<<EOF' here-document not contained within command substitution\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// What the sentence quotes, and which shapes reach it at all.
//
// Measured the same day, each file its own run. The token is the operator with
// the delimiter's quoting off: a `-`, a descriptor in front of the operator
// and the quotes on the word are none of them written, and a delimiter with
// blanks in it keeps them.
func TestTheUncontainedHereDocumentRefusalQuotesTheOperatorAndItsDelimiter(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the plain operator", "echo $(cat <<EOF)\nbody\nEOF\n",
			"syntax error at line 1: `<<EOF' here-document not contained within command substitution",
		},
		{
			"the stripping operator", "echo $(cat <<-EOF)\nbody\nEOF\n",
			"syntax error at line 1: `<<EOF' here-document",
		},
		{
			"a quoted delimiter", "echo $(cat <<\"EOF\")\nbody\nEOF\n",
			"syntax error at line 1: `<<EOF' here-document",
		},
		{
			"an escaped delimiter", "echo $(cat <<\\EOF)\nbody\nEOF\n",
			"syntax error at line 1: `<<EOF' here-document",
		},
		{
			"a descriptor in front of the operator", "echo $(cat 3<<EOF)\nbody\nEOF\n",
			"syntax error at line 1: `<<EOF' here-document",
		},
		{
			"a delimiter with blanks in it", "echo $(cat <<'E O F')\nbody\nE O F\n",
			"syntax error at line 1: `<<E O F' here-document",
		},
		{
			// The operator's own line and not the substitution's opener.
			"an operator on a later line", "echo $(echo a\ncat <<EOF)\nbody\nEOF\n",
			"syntax error at line 2: `<<EOF' here-document",
		},
		{
			// The inner construct is what is refused, and it is reported as a
			// line that was read rather than as a line that was running.
			"a substitution inside another", "echo $(echo $(cat <<EOF))\nbody\nEOF\n",
			"syntax error at line 1: `<<EOF' here-document",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := refuseAsKsh(t, c.src); !strings.Contains(got, c.want) {
				t.Errorf("%q = %q, want %q in it", c.src, got, c.want)
			}
		})
	}
}

// And the shapes this shell accepts, which are what a rule keyed on "a
// here-document written inside parentheses" would refuse wrongly.
//
// Measured the same day: the body inside the parentheses is the ordinary
// reading, a here-string is not a here-document, the backquoted spelling takes
// its body from the whole input, and a process substitution is accepted — that
// shell reads *its* body from the lines after the enclosing command, which is
// a reading nothing here has and is not this refusal.
func TestTheUncontainedHereDocumentRefusalLeavesTheOtherShapesAlone(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a body inside the parentheses", "echo $(cat <<EOF\nbody\nEOF\n)\n", "body"},
		{"a delimiter carrying the closer", "echo $(cat <<EOF\nbody\nEOF)\n", "body"},
		{"a here-string", "echo $(cat <<<word)\n", "word"},
		{"the backquoted spelling", "echo `cat <<EOF`\nbody\nEOF\n", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := syntax.Parse(c.src, ksh.Dialect()); err != nil {
				t.Fatalf("%q = %v, want it read", c.src, err)
			}
			out, _ := runKshWithTools(t, c.src)
			if c.want != "" && !strings.Contains(out, c.want) {
				t.Errorf("%q = %q, want %q in it", c.src, out, c.want)
			}
		})
	}
}
