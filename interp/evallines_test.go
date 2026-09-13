// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Where the lines of `eval`'s text are. Two readings of identical text with no
// subset between them — see Semantics.EvalTextContinuesTheCallersLines — so
// this names the axis and no shell.
//
// **The obvious probe cannot decide it.** Spread an `eval` over several
// physical lines and "the physical line the failing text sits on" and "the
// caller's line plus the text's line, less one" are the same number, so a
// green test proves nothing. Every source here therefore holds the whole
// `eval` on one physical line with a newline carried in through a parameter,
// which separates all three candidate answers:
//
//	nl='          line 1, and the quoted newline makes line 2 its end
//	'
//	eval "echo x${nl}echo L=\$LINENO"     the eval word is on line 3
//
// The read is on the text's line 2, so the text's own numbering says 2, the
// physical line says 3, and continuing the caller's lines says 4.
const evalOnOneLine = "nl='\n'\n"

func TestEvalTextContinuesTheCallersLinesIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    Answer
		want string
	}{
		// 3 + 2 - 1, and not the 3 the physical reading would give.
		{"continued", Yes, "L=4"},
		{"from one", No, "L=2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := evalOnOneLine + `eval "echo x${nl}echo L=\$LINENO"` + "\n"
			out, _ := run(t, src, func(r *Runner) {
				s := *r.Semantics
				s.EvalTextContinuesTheCallersLines = tc.a
				r.Semantics = &s
			})
			if got := strings.TrimSpace(out); got != "x\n"+tc.want {
				t.Errorf("%v: got %q, want %q", tc.a, got, "x\n"+tc.want)
			}
		})
	}
}

// The offset is the text's and does not outlive it: the line after the `eval`
// is the file's own line under either answer, and so is a *later* `eval`.
func TestTheEvalOffsetEndsWithTheText(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		src := evalOnOneLine + `eval "echo x"` + "\necho after=$LINENO\n"
		out, _ := run(t, src, func(r *Runner) {
			s := *r.Semantics
			s.EvalTextContinuesTheCallersLines = a
			r.Semantics = &s
		})
		if want := "x\nafter=4"; strings.TrimSpace(out) != want {
			t.Errorf("%v: got %q, want %q", a, strings.TrimSpace(out), want)
		}
	}
}

// A function called from inside the text is not part of the text: its body's
// lines are its own however the caller was numbering. Measured 2026-09-12,
// bash and zsh both report the body's own line for a function called from
// inside an `eval` and from inside a command substitution — and this shell
// reported the *caller's* offset for the substitution case, which is the bug
// the same field carries either way (#2462).
func TestALineOffsetDoesNotReachAFunctionBody(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		src := "f() { echo \"in f L=$LINENO\"; }\n" + evalOnOneLine +
			`eval "echo x${nl}f"` + "\nx=$(f); echo \"sub $x\"\n"
		out, _ := run(t, src, func(r *Runner) {
			s := *r.Semantics
			s.EvalTextContinuesTheCallersLines = a
			r.Semantics = &s
		})
		want := "x\nin f L=1\nsub in f L=1"
		if strings.TrimSpace(out) != want {
			t.Errorf("%v: got %q, want %q", a, strings.TrimSpace(out), want)
		}
	}
}

// A sourced file's lines are its own in every shell in the panel, so no
// offset reaches one — which is not what this did: with a `.` inside a
// command substitution the file's first line read as the substitution's line.
func TestNoLineOffsetReachesASourcedFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "inc.sh", "echo \"inc L=$LINENO\"\n")
	src := "echo pad\necho pad\nx=$(. ./inc.sh); echo \"sub: $x\"\n. ./inc.sh\n"
	out, _ := sourceRun(t, dir, src, permissive(), Diagnostics{})
	if want := "sub: inc L=1\ninc L=1"; !strings.Contains(out, want) {
		t.Errorf("got %q, want it to contain %q", out, want)
	}
}
