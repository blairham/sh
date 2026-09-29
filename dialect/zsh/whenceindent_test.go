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
	// And the letter that is still missing is still refused as missing, which
	// is what keeps "unimplemented" meaning something: a script can tell a
	// shell that lacks a feature from a typo.
	t.Run("the letter this shell has not got", func(t *testing.T) {
		out, st, errs := runZshSplit(t, dir, fn+"which -m f\n")
		want := "zsh:which:2: -m is not implemented yet\n"
		if out != "" || errs != want || st != 1 {
			t.Errorf("out %q err %q status %d, want %q at 1", out, errs, st, want)
		}
	})
}
