// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// `functions -x num` writes each level of the listing's structure as num
// spaces instead of as the tab this shell adds by default. Measured
// 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), run `-f` with
// `env -u FPATH` over a script file (#4442).
//
// It is a property of the listing rather than of the function, so the
// discriminator is a body with *two* levels in it: a one-level body cannot
// tell "num spaces a level" from "num spaces once".
const twoLevelFunction = "g() { if true; then print a; print b; fi; }\n"

func TestFunctionsIndentWritesTheLevelsAsSpaces(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), twoLevelFunction+`functions -x 2 g`)
	want := "g () {\n  if true\n  then\n    print a\n    print b\n  fi\n}\n"
	if out != want || st != 0 {
		t.Errorf("functions -x 2 = %q (status %d), want %q", out, st, want)
	}
	// The control: with no letter the listing is tabs, which is what says
	// the letter changed something rather than the printer always having
	// written spaces.
	out, st = runZsh(t, t.TempDir(), twoLevelFunction+`functions g`)
	want = "g () {\n\tif true\n\tthen\n\t\tprint a\n\t\tprint b\n\tfi\n}\n"
	if out != want || st != 0 {
		t.Errorf("functions = %q (status %d), want %q", out, st, want)
	}
}

// Zero suppresses the indentation altogether, and so does anything below it.
func TestFunctionsIndentOfNoneAtAll(t *testing.T) {
	flush := "g () {\nif true\nthen\nprint a\nprint b\nfi\n}\n"
	for _, src := range []string{`functions -x 0 g`, `functions -x0 g`, `functions -x -1 g`} {
		out, st := runZsh(t, t.TempDir(), twoLevelFunction+src)
		if out != flush || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", src, out, st, flush)
		}
	}
}

// Where the number comes from, and the two refusals — which are two different
// sentences for what reads as one fault. A word that is not a number is the
// number's own complaint; no word at all is the option reader's.
func TestFunctionsIndentTakesItsNumber(t *testing.T) {
	two := "g () {\n  if true\n  then\n    print a\n    print b\n  fi\n}\n"
	for _, c := range []struct{ src, want string }{
		// Attached, and attached behind another letter's bundle.
		{`functions -x2 g`, two},
		{`functions -mx 2 'g*'`, two},
		// The letter ends its bundle, so the rest of its own word is the
		// number and a letter there is not an option.
		{`functions -xm 2 'g*'`, "zsh:functions:2: number expected after -x\n"},
		{`functions -x2m 'g*'`, "zsh:functions:2: number expected after -x\n"},
		{`functions -x abc g`, "zsh:functions:2: number expected after -x\n"},
		{`functions -x`, "zsh:functions:2: argument expected: -x\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), twoLevelFunction+c.src)
		if out != c.want {
			t.Errorf("%s = %q, want %q", c.src, out, c.want)
		}
	}
}

// The letter is accepted rather than named as missing, which is the invariant
// the paired tables carry: a letter in both is refused while it works.
func TestFunctionsIndentLetterIsNotAlsoCalledMissing(t *testing.T) {
	if !strings.ContainsRune(zsh.Semantics().FunctionsOptions, 'x') {
		t.Error("FunctionsOptions does not spell -x")
	}
	if got := zsh.Diagnostics().UnimplementedOptionLetters["functions"]; strings.ContainsRune(got, 'x') {
		t.Errorf("UnimplementedOptionLetters[functions] = %q, which still claims -x is missing", got)
	}
}
