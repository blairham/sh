// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `-x num` is how wide one level of a printed function body is, and it belongs
// to four names rather than one.
//
// `functions -x2 f` already wrote a body indented by two; `whence`, `which` and
// `where` refused the letter as unimplemented, which is what
// `A04redirect.ztst` stops on under `Regression test for off-by-one in varid
// check`.
//
// Measured 2026-09-29 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`; `go version -m`
// reports *not a Go executable*), script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME and standard input on the null
// device.
//
// **The number may be attached or the next word**, which is why the reader runs
// before the letters are parsed: `which -x 2 f` is three words and two of them
// belong to the option. A grid of bare `-x` rows cannot see that, so the
// attached and detached spellings are both here from the start.
func TestTheIndentLetterBelongsToEveryNameThatPrintsABody(t *testing.T) {
	dir := t.TempDir()
	const fn = "f(){ print hi }\n"
	for _, tc := range []struct{ name, src, want string }{
		{"which, attached", fn + "which -x2 f\n", "f () {\n  print hi\n}\n"},
		{"which, detached", fn + "which -x 2 f\n", "f () {\n  print hi\n}\n"},
		{"which, four", fn + "which -x4 f\n", "f () {\n    print hi\n}\n"},
		{"which, zero suppresses it", fn + "which -x0 f\n", "f () {\nprint hi\n}\n"},
		{"whence, which needs -f for a body", fn + "whence -x2 -f f\n", "f () {\n  print hi\n}\n"},
		{"where", fn + "where -x2 f\n", "f () {\n  print hi\n}\n"},
		{"functions, which had it already", fn + "functions -x2 f\n", "f () {\n  print hi\n}\n"},
		// The letter may stand anywhere among the others.
		{"before another letter", fn + "which -x2 -a f\n", "f () {\n  print hi\n}\n"},
		{"and after one", fn + "which -a -x2 f\n", "f () {\n  print hi\n}\n"},
		// Every level of structure moves, not just the first.
		{
			"a nested body", "f(){ if true; then print hi; fi }\nwhich -x2 f\n",
			"f () {\n  if true\n  then\n    print hi\n  fi\n}\n",
		},
		// Bundled with another letter, which says the reader takes the
		// letter out of a word rather than matching one.
		{"bundled after another", fn + "which -ax2 f\n", "f () {\n  print hi\n}\n"},
		{"and under whence, bundled with -f", fn + "whence -fx2 f\n", "f () {\n  print hi\n}\n"},
		// The control: with no letter the body keeps the tab it had.
		{"no letter is a tab", fn + "which f\n", "f () {\n\tprint hi\n}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A missing or unreadable number is one sentence, and it is the same sentence
// under all four names — which is the whole reason the reader is shared rather
// than written again for the three that lacked it.
func TestTheIndentLetterNeedsANumber(t *testing.T) {
	dir := t.TempDir()
	const fn = "f(){ print hi }\n"
	for _, tc := range []struct{ name, src, want string }{
		{"which, a letter after it", fn + "which -xa f\n", "zsh:which:2: number expected after -x\n"},
		{"which, nothing after it", fn + "which -x f\n", "zsh:which:2: number expected after -x\n"},
		{"which, a separator after it", fn + "which -x -- f\n", "zsh:which:2: number expected after -x\n"},
		{"whence", fn + "whence -xa -f f\n", "zsh:whence:2: number expected after -x\n"},
		{"where", fn + "where -xa f\n", "zsh:where:2: number expected after -x\n"},
		{"functions, which said it already", fn + "functions -xa f\n", "zsh:functions:2: number expected after -x\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, dir, tc.src)
			if out != "" || errs != tc.want || st != 1 {
				t.Errorf("out %q err %q status %d, want %q at 1", out, errs, st, tc.want)
			}
		})
	}
	// `-m` used to be refused here as the letter still missing; it is built
	// now (#5230), and the indent reaches the bodies it lists. Measured on
	// zsh 5.9.2: `which -x2 -m f` and `which -mx2 f` both write the body
	// indented by two.
	for _, src := range []string{"which -x2 -m f\n", "which -mx2 f\n"} {
		out, st, errs := runZshSplit(t, dir, fn+src)
		if want := "f () {\n  print hi\n}\n"; out != want || errs != "" || st != 0 {
			t.Errorf("%q: out %q err %q status %d, want %q at 0", src, out, errs, st, want)
		}
	}
}

// **The arrangement is for the length of one command**, which is the half a
// caller can get wrong without any row noticing: setting the indent and not
// putting it back leaves every later listing in the shell wearing it.
//
// Measured: `which -x2 f; which f` writes the body indented by two and then the
// body indented by a tab, in the reference and here. The second listing is the
// whole of the test — the first is there to set the trap.
func TestTheIndentDoesNotOutliveItsCommand(t *testing.T) {
	dir := t.TempDir()
	const fn = "f(){ print hi }\n"
	const two = "f () {\n  print hi\n}\n"
	const tab = "f () {\n\tprint hi\n}\n"
	for _, tc := range []struct{ name, src, want string }{
		{"the same name twice", fn + "which -x2 f\nwhich f\n", two + tab},
		{"and across the names", fn + "which -x2 f\nfunctions f\n", two + tab},
		{"the other way round", fn + "functions -x2 f\nwhich f\n", two + tab},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
