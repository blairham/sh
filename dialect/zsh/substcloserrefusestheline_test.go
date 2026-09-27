// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A `$( … )` body this shell refuses at the **closing parenthesis** refuses
// the line it is written on, so the commands written in front of the
// substitution never run (#4859).
//
// The construct did close — the `)` is where the grammar stopped — so nothing
// about the *read* changes and every wording counted from it is untouched.
// What changes is that the line has an answer, where before the body was not
// looked at again until the word was expanded, which is after the text in
// front of it had run.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh`,
// `zsh 5.9.2 (aarch64-apple-darwin25.4.0)`, each case the one-line script file
// `echo b; v=$(X); echo a` run as `zsh ./s.sh` with standard input on the null
// device — and each measured again with `setopt shortloops` and with
// `unsetopt shortloops` on a line above it, which moves none of these
// eighteen columns.
func TestABodyRefusedAtTheCloserRefusesTheLine(t *testing.T) {
	for _, body := range []string{
		// The six the reference refuses unconditionally, which is the
		// measurement #4859 was filed from.
		"for", "case x", "{", "select", "repeat", "echo x |",
		// And the three `if` shapes the short-body option does not reach:
		// an `if` past its `then`, and an unfinished `if` inside something
		// else. What decides is the outermost construct, so these go with
		// the six rather than with the option's own four.
		"if true; then", "if true; then if", "while true; do if",
	} {
		t.Run(body, func(t *testing.T) {
			if out := prefixOfARefusedLine(t, body); out != "" {
				t.Errorf("$(%s) wrote %q, so the line ran", body, out)
			}
		})
	}
}

// And the bodies it does not, which is what says the rule is about the
// refusal rather than about the substitution: each of these writes `b` in the
// reference too.
func TestABodyTheReferenceDefersLeavesTheLineRunning(t *testing.T) {
	for _, body := range []string{
		// The short-body option's own population, with the option on — which
		// is this preset's default. TestTheShortLoopsOptionDecidesIt is where
		// it is moved.
		"if", "if true", "if if true", "if true; then :; elif",
		// The and-or leniency, which is no refusal at all.
		"echo &&", "echo ||", "!", "echo ;",
		// And a body with nothing wrong in it.
		"echo hi",
	} {
		t.Run(body, func(t *testing.T) {
			// The prefix and not the whole line: a body the *word* refuses
			// when it is expanded still ends the script there, so `a` is
			// written only where nothing was wrong with the body at all.
			// What this asks is the half that moved — whether the text in
			// front of the substitution ran.
			if out := prefixOfARefusedLine(t, body); !strings.HasPrefix(out, "b\n") {
				t.Errorf("$(%s) wrote %q, want the line in front of it to have run", body, out)
			}
		})
	}
}

// The places a refused body still does **not** refuse the line, pinned at
// today's behavior so that a change which moved them would have to say so.
//
// The reference refuses the line in every one of these as well, and this
// shell writes `b` first. It is left where it is because what the reference
// writes second is the **enclosing** construct's complaint — `unmatched "`
// for a quote, `closing brace expected` for a parameter expansion, the whole
// line for a nesting — and this shell reaches those by reading the body when
// the word is expanded. Refusing the line here instead would write the
// substitution's complaint where the quote's belongs, which is a wrong answer
// in place of a late one. Measured 2026-09-27 on zsh 5.9.2; the second message
// is byte-identical to the reference in all five rows, and only the `b` is
// not.
//
// The last row is a different case and is measured rather than grouped with
// them: a function body that never came is the one refusal at the closer this
// shell numbers from the parentheses, and the closed and unclosed routes to it
// are numbered differently — see Diagnostics.bodyRefusalWrittenFirst.
func TestARefusedBodyThatDoesNotReachTheLine(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"inside a double quote", `echo b; echo "x $(for) y"; echo a`},
		{"inside a quoted parameter expansion", `echo b; echo "${x:-$(for)}"; echo a`},
		{"inside an unquoted parameter expansion", `echo b; echo ${x:-$(for)}; echo a`},
		{"inside another substitution's body", `echo b; v=$(echo $(for)); echo a`},
		{"inside a process substitution's body", `echo b; cat <(v=$(for)); echo a`},
		{"a function body that never came", `echo b; v=$(echo hi; foo()); echo a`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs := runTheLine(t, c.src+"\n")
			if !strings.HasPrefix(out, "b\n") {
				t.Errorf("wrote %q, want the line in front of it still to have run", out)
			}
			if !strings.Contains(errs, "`)'") {
				// Without this the row passes for a shell that accepted the
				// body outright, which is the answer nobody wants.
				t.Errorf("wrote %q, with no refusal of the body in it", errs)
			}
		})
	}
}

// prefixOfARefusedLine is what `echo b; v=$(body); echo a` writes on standard
// output, which is `b` where the line ran and nothing where it did not.
func prefixOfARefusedLine(t *testing.T, body string) string {
	t.Helper()
	out, _ := runTheLine(t, "echo b; v=$("+body+"); echo a\n")
	return out
}

func runTheLine(t *testing.T, src string) (out, errs string) {
	t.Helper()
	var o, e strings.Builder
	r := preset.Runner(dialecttest.Base{
		Stdout: &o, Stderr: &e, Dir: t.TempDir(),
		// `cat` is an external command here, for the one row that writes a
		// process substitution: this shell does not run it itself, and a row
		// whose command was not found would pass for the wrong reason.
		Vars: map[string]string{"PATH": "/usr/bin:/bin"},
	})
	preset.RunLinesOn(t, r, src)
	return o.String(), e.String()
}
