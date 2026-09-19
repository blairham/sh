// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package printer_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A `function` keyword's name list is greedy, so a body written beside it is
// read back as more names. This pass wrote the blank whatever the body was:
// `function foo` with `echo hi` under it came out as `function foo echo hi`,
// which defines **three** functions — `foo`, `echo` and `hi`, each with
// whatever followed as its body — and runs none of them. Status 0 both ways,
// so nothing downstream could tell (#3746).
//
// syntax.Print had the guard and this printer never had it, which is why the
// rule now lives in one place both ask —
// syntax.FunctionKeywordBodyNeedsItsOwnLine — and why this test grades the
// answer rather than the call.
//
// Every row is measured three ways: the formatted text keeps the body off the
// header's line, the tree is the same program, and the script *runs* the
// same. The third is the one that matters and the first is the one that
// works with no shell installed — a round trip that only asks whether the
// output parses passes on every row here, since `function foo echo hi` is a
// perfectly good program.
func TestAKeywordDeclarationsNonBraceBodyTakesItsOwnLine(t *testing.T) {
	// Measured against zsh 5.9.2 on 2026-09-19, each body first on the line
	// after `function foo` and then beside it. Only a brace group survives
	// the move: `( echo hi )` beside the name is `unknown file attribute`,
	// the loops and `case` are a parse error near their own keyword, and
	// `echo hi` is the silent one — three functions defined, nothing run,
	// status 0.
	for _, tc := range []struct {
		name string
		body string
		// firstLine is what the formatted script opens with. `function foo`
		// alone is the break this issue is about; anything longer is a body
		// that ended the name list itself and so may stand beside it.
		firstLine string
	}{
		{name: "simple-command", body: "echo hi", firstLine: "function foo"},
		{name: "declaration", body: "function bar { echo hi; }", firstLine: "function foo"},
		{name: "subshell", body: "( echo hi )", firstLine: "function foo"},
		{name: "if", body: "if true; then echo hi; fi", firstLine: "function foo"},
		{name: "while", body: "while false; do echo hi; done", firstLine: "function foo"},
		{name: "for", body: "for i in 1; do echo hi; done", firstLine: "function foo"},
		{name: "case", body: "case x in x) echo hi ;; esac", firstLine: "function foo"},
		{name: "repeat", body: "repeat 1; do echo hi; done", firstLine: "function foo"},
		{name: "brace-group", body: "{ echo hi; }", firstLine: "function foo { echo hi; }"},
		// An and-or list is not one command, so the grammar that reads a
		// body this way wraps it in a group — see
		// syntax.Dialect.FunctionKeywordBodyIsAnAndOrList. The braces are
		// the tree's, and they are what lets these two stay on the line.
		{name: "arith-andor", body: "(( 1 )) && echo hi", firstLine: "function foo { (( 1 )) && echo hi; }"},
		{name: "test-clause-andor", body: "[[ -n x ]] && echo hi", firstLine: "function foo { [[ -n x ]] && echo hi; }"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, st := dialect(true), style(true)
			// `${#functions}` is the discriminator and the call is the
			// control: a row that only ran `foo` would pass for a formatter
			// that defined one extra function whose body never ran.
			src := "function foo\n" + tc.body + "\nprintf 'n=%d ' ${#functions}\nfoo\n"
			out := format(t, src, d, st)

			if header, _, _ := strings.Cut(out, "\n"); header != tc.firstLine {
				t.Errorf("the first line is %q, want %q\noutput:\n%s", header, tc.firstLine, out)
			}

			fIn, err := syntax.Parse(src, d)
			if err != nil {
				t.Fatalf("input does not parse: %v", err)
			}
			fOut, err := syntax.Parse(out, d)
			if err != nil {
				t.Fatalf("output does not parse: %v\noutput:\n%s", err, out)
			}
			if why, ok := syntax.SameProgram(fIn, fOut); !ok {
				t.Errorf("the program changed at %s\ninput:\n%s\noutput:\n%s", why, src, out)
			}
			if again := format(t, out, d, st); again != out {
				t.Errorf("formatting is not a fixed point\nonce:\n%s\ntwice:\n%s", out, again)
			}

			if _, err := exec.LookPath("zsh"); err != nil {
				t.Skipf("zsh not installed, so only the layout and the tree were graded")
			}
			beforeOut, beforeErr, beforeStatus := run(t, "zsh", src)
			// The row must be one where the source defines exactly one
			// function and calls it — otherwise the comparison below is two
			// identical refusals agreeing with each other, which is the
			// non-discriminating probe this campaign keeps writing.
			if !strings.HasPrefix(beforeOut, "n=1 ") || beforeStatus != 0 {
				t.Fatalf("this row cannot grade the formatter: the source itself gives %q, %q, status %d",
					beforeOut, beforeErr, beforeStatus)
			}
			afterOut, afterErr, afterStatus := run(t, "zsh", out)
			if beforeOut != afterOut || beforeErr != afterErr || beforeStatus != afterStatus {
				t.Errorf("behavior changed\nformatted:\n%s\nstdout %q -> %q\nstderr %q -> %q\nstatus %d -> %d",
					out, beforeOut, afterOut, beforeErr, afterErr, beforeStatus, afterStatus)
			}
		})
	}
}

// The parenthesized headers do not take the break, because `()` ends the name
// list itself: `foo() echo hi` and `function foo() echo hi` each define one
// function. The first keeps its body on the header's line; the second cannot,
// because the tree does not record the hybrid's parentheses — see
// syntax.FuncDecl — so nothing reading the tree can part `function foo()`
// from `function foo`, and only the break is right for both.
//
// Both are graded by running them, which is what says the choice is a layout
// one rather than a rewrite.
func TestAParenthesizedHeaderKeepsItsOwnShape(t *testing.T) {
	for _, tc := range []struct {
		name, src, wantFirstLine string
	}{
		{
			name:          "parens",
			src:           "foo()\necho hi\nprintf 'n=%d ' ${#functions}\nfoo\n",
			wantFirstLine: "foo() echo hi",
		},
		{
			name:          "keyword-and-parens",
			src:           "function foo()\necho hi\nprintf 'n=%d ' ${#functions}\nfoo\n",
			wantFirstLine: "function foo()",
		},
		{
			name:          "parens-brace-body",
			src:           "foo() { echo hi; }\nprintf 'n=%d ' ${#functions}\nfoo\n",
			wantFirstLine: "foo() { echo hi; }",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, st := dialect(true), style(true)
			out := format(t, tc.src, d, st)
			if first, _, _ := strings.Cut(out, "\n"); first != tc.wantFirstLine {
				t.Errorf("first line is %q, want %q\noutput:\n%s", first, tc.wantFirstLine, out)
			}
			if _, err := exec.LookPath("zsh"); err != nil {
				t.Skipf("zsh not installed, so only the layout was graded")
			}
			beforeOut, beforeErr, beforeStatus := run(t, "zsh", tc.src)
			if !strings.HasPrefix(beforeOut, "n=1 ") || beforeStatus != 0 {
				t.Fatalf("this row cannot grade the formatter: the source itself gives %q, %q, status %d",
					beforeOut, beforeErr, beforeStatus)
			}
			afterOut, afterErr, afterStatus := run(t, "zsh", out)
			if beforeOut != afterOut || beforeErr != afterErr || beforeStatus != afterStatus {
				t.Errorf("behavior changed\nformatted:\n%s\nstdout %q -> %q\nstderr %q -> %q\nstatus %d -> %d",
					out, beforeOut, afterOut, beforeErr, afterErr, beforeStatus, afterStatus)
			}
		})
	}
}
