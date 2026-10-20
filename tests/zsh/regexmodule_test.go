// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `zsh/regex`: one infix condition and nothing else —
// `[[ subject -regex-match expression ]]`, a POSIX extended regular
// expression. The module was refused outright and the condition was `unknown
// condition: -regex-match` (#4739).
//
// Measured 2026-09-26 on zsh 5.9.2 (`/opt/homebrew/bin/zsh`,
// aarch64-apple-darwin25.4.0), run `-f` under `env -i PATH=/usr/bin:/bin`.
// `go version -m` says *not a Go executable* for it and
// `github.com/blairham/sh/cmd/zsh` for ours, so these are two programs.

func runRegex(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src+"\n")
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// **The condition is `=~` under another name**, and this is the grid that
// says so: the two spellings are run side by side over the eleven
// expressions, and every row has to agree on the status, on `$MATCH`, on
// `$match`, on `$MBEGIN` and on `$MEND`.
//
// That is the measurement the implementation rests on — in the reference the
// two were byte-identical over exactly these rows — so a second engine
// written behind the condition would show up here and nowhere else.
func TestTheRegexConditionIsTheRegexOperatorUnderAnotherName(t *testing.T) {
	for _, expr := range []string{
		`b`, `^b`, `a|z`, `a+`, `(a)(b)(c)`, `(z)?(b)`, `a{1,2}`, `\d`, `B`, `x`, `a.c`,
	} {
		t.Run(expr, func(t *testing.T) {
			const report = `; print "r=$? M=[$MATCH] m=[${(j:,:)match}] MB=$MBEGIN ME=$MEND"`
			out, _ := runRegex(t, "[[ abc -regex-match '"+expr+"' ]]"+report+"\n"+
				"[[ abc =~ '"+expr+"' ]]"+report)
			lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
			if len(lines) != 2 {
				t.Fatalf("out = %q, want two lines", out)
			}
			if lines[0] != lines[1] {
				t.Errorf("-regex-match wrote %q where =~ wrote %q", lines[0], lines[1])
			}
		})
	}
}

// The answers themselves, so the grid above cannot pass by the two spellings
// being broken in the same way. Each row is the reference's, measured.
//
// The last two are the pair that says the match is unanchored and
// leftmost-longest rather than leftmost-first: `a|z` against `abc` takes `a`
// at position 1, and `a.c` takes the whole subject.
func TestWhatTheRegexConditionAnswers(t *testing.T) {
	for _, c := range []struct{ expr, want string }{
		{`b`, "r=0 M=[b] m=[] MB=2 ME=2\n"},
		{`^b`, "r=1 M=[] m=[] MB= ME=\n"},
		{`(a)(b)(c)`, "r=0 M=[abc] m=[a,b,c] MB=1 ME=3\n"},
		// A group that took no part is an empty element and the ones after
		// it keep their places, which is what `$#match` being 2 says.
		{`(z)?(b)`, "r=0 M=[b] m=[,b] MB=2 ME=2\n"},
		// POSIX ERE and not a PCRE: `\d` is not a class here and matches
		// nothing. It is the row that separates this module from #4737's.
		{`\d`, "r=1 M=[] m=[] MB= ME=\n"},
		{`B`, "r=1 M=[] m=[] MB= ME=\n"},
		{`a|z`, "r=0 M=[a] m=[] MB=1 ME=1\n"},
		{`a.c`, "r=0 M=[abc] m=[] MB=1 ME=3\n"},
	} {
		t.Run(c.expr, func(t *testing.T) {
			out, _ := runRegex(t, "[[ abc -regex-match '"+c.expr+
				`' ]]; print "r=$? M=[$MATCH] m=[${(j:,:)match}] MB=$MBEGIN ME=$MEND"`)
			if out != c.want {
				t.Errorf("out = %q, want %q", out, c.want)
			}
		})
	}
}

// The prefix spelling is not this condition written short: `[[ -regex-match
// abc ]]` is `unknown condition: -regex-match` in the reference too, measured,
// so an operator with nothing in front of it falls through to the refusal it
// always had.
func TestTheRegexConditionNeedsAnOperandInFrontOfIt(t *testing.T) {
	out, st := runRegex(t, `[[ -regex-match abc ]]; print never`)
	if want := "zsh:1: unknown condition: -regex-match\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
}

// And the module: it loads, it lists its one feature, and the letter is a
// capital `C` — which is how the reference's own listing separates an infix
// condition from a completion one, `+c:prefix` against `+C:regex-match`. The
// entry lands with the condition, which matters most for this kind of
// feature: a condition has no call site to refuse at.
func TestTheRegexModuleLoadsWithItsOneCondition(t *testing.T) {
	out, st := runRegex(t, "zmodload zsh/regex\nprint l=$?\nzmodload -lF zsh/regex\nzmodload")
	want := "l=0\n+C:regex-match\nzsh/main\nzsh/regex\n"
	if out != want || st != 0 {
		t.Errorf("out = %q (status %d), want %q", out, st, want)
	}
	// The control that says the capital letter is doing work: the
	// completion conditions keep the small one, so a gate that answered
	// both from one registry would print this row wrong.
	out, _ = runRegex(t, "zmodload zsh/complete\nzmodload -lF zsh/complete")
	if !strings.Contains(out, "+c:prefix") {
		t.Errorf("zsh/complete's listing = %q, want a small-c condition in it", out)
	}
}
