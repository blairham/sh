// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// Four wording rows from the bounded error sample #3088 ran, all of them this
// column's alone, and all measured 2026-09-17 inside the pinned image
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0, against `cmd/ash` cross-compiled into the same container
// (#3141, #3239).

// `ulimit` writes its bad-option refusal bare and reports 1, where `unset -Z`,
// `read -Z` and `trap -Z` in the same shell each carry the script and the line
// and report 2. So the exception is this builtin's rather than a house style.
func TestUlimitWritesItsBadOptionBareAndReportsOne(t *testing.T) {
	out, status := run(t, "ulimit -Z\n")
	if want := "ulimit: unrecognized option: Z\n"; out != want {
		t.Errorf("got %q, want exactly %q — no script, no line", out, want)
	}
	if status != 1 {
		t.Errorf("status %d, want 1", status)
	}
	// The control: the same shape of refusal from another builtin keeps the
	// location and the 2.
	out, status = run(t, "unset -Z\n")
	if !strings.Contains(out, "unset: line 1:") || status != 2 {
		t.Errorf("unset -Z = %q at %d, want the located refusal at 2", out, status)
	}
}

// The directive a refused conversion names is written without the length
// modifiers this shell just read past, and the modifier run itself is a run of
// `h`, `l`, `z` and `L` — neither of the two answers the other columns hold.
func TestPrintfNamesTheDirectiveWithoutItsLengthModifiers(t *testing.T) {
	for _, tc := range []struct {
		format string
		want   string
		why    string
	}{
		{"%z", "%: invalid format", "the row the issue was filed on"},
		{"%l", "%: invalid format", "the same for a C89 letter"},
		{"%hhz", "%: invalid format", "and for a run of them"},
		{"%5.2lz", "%5.2: invalid format", "the width and precision survive"},
		{"%+ #0z", "%+ #0: invalid format", "and so do the flags"},
		{"%zq", "%q: invalid format", "the control: a conversion character survives"},
		{"%q", "%q: invalid format", "and one with no modifier in front is untouched"},
	} {
		out, status := run(t, "printf '"+tc.format+"' 1\n")
		if !strings.Contains(out, tc.want) || status != 1 {
			t.Errorf("printf %q = %q at %d, want %q at 1 — %s",
				tc.format, out, status, tc.want, tc.why)
		}
	}
	// And the modifiers this shell really takes, which is what makes `%z`
	// reach the missing-conversion complaint at all.
	for _, format := range []string{"%zd", "%ld", "%hd", "%Ld", "%hhd", "%lld"} {
		out, status := run(t, "printf '"+format+"' 1\n")
		if out != "1" || status != 0 {
			t.Errorf("printf %q = %q at %d, want `1` at 0", format, out, status)
		}
	}
	// `j` and `t` are not modifiers here, so they arrive as the conversion
	// character and are refused — which is what parts this answer from C99's.
	for _, format := range []string{"%jd", "%td"} {
		out, status := run(t, "printf '"+format+"' 1\n")
		if !strings.Contains(out, ": invalid format") || status != 1 {
			t.Errorf("printf %q = %q at %d, want a refusal at 1", format, out, status)
		}
	}
}

// A `\x` with no hexadecimal digit after it says nothing here, where bash —
// the only other column that reaches the reading — writes a complaint. The
// escape stands and the status is 0 in both.
func TestPrintfSaysNothingAboutAMissingHexDigit(t *testing.T) {
	for _, src := range []string{`printf 'a\xZb'`, `printf '%b' 'a\xZb'`} {
		out, status := run(t, src+"\n")
		if out != `a\xZb` || status != 0 {
			t.Errorf("%s = %q at %d, want `a\\xZb` at 0 and nothing on stderr",
				src, out, status)
		}
	}
	// The control: a digit that is there is still read.
	if out, _ := run(t, `printf 'a\x41b'`+"\n"); out != "aAb" {
		t.Errorf("got %q, want `aAb`", out)
	}
}

// `eval`'s text continues the caller's lines here, and the caller's first line
// on `-c` is **line 0** — so an `eval` on the second line of a command string
// numbers its text from 1, and one on the first line numbers it from 0. The
// guard that asked the axis read the second line as the first and left the
// offset at nothing.
func TestEvalLinesCountFromTheRoutesOwnFirstLine(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
		why  string
	}{
		{`eval "echo )"`, "eval: line 0:", "the eval is on the route's first line, which is 0"},
		{"echo one\n" + `eval "echo )"`, "eval: line 1:", "and on its second, which is 1"},
		{"eval \"echo a\necho )\"", "eval: line 1:", "the text's second line, from an eval on line 0"},
		{`eval "nosuchcmd"`, "eval: line 0:", "a run-time failure reads the same counter"},
		{"echo one\n" + `eval "nosuchcmd"`, "eval: line 1:", "and moves with it"},
	} {
		if out := runCommandString(t, tc.src); !strings.Contains(out, tc.want) {
			t.Errorf("got %q, want %q — %s", out, tc.want, tc.why)
		}
	}
	// The control: a script file is 1-based, so nothing here moved it.
	f := preset.Parse(t, "echo z\n"+`eval "echo )"`+"\n")
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{
		Name: "ash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
		Stdout: &buf, Stderr: &buf, Route: interp.RouteScriptFile,
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(buf.String(), "eval: line 2:") {
		t.Errorf("got %q, want `eval: line 2:` on a script file", buf.String())
	}
}
