// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// `promptvars` decides whether a prompt's value is expanded each time it is
// drawn. It is on with nothing said, and it gates the **expansion** pass only
// — the backslash language runs either way.
//
// Measured 2026-09-23 on bash 5.3.15 through a pty, `bash --norc --noprofile
// -i` under `script`, one prompt per run:
//
//	PS1='[$x]> ' with x=XVAL     on `[XVAL]> `          off `[$x]> `
//	PS1='[$(echo SUB)]> '        on `[SUB]> `           off `[$(echo SUB)]> `
//	PS1='[\u]> '                 on `[root]> `          off `[root]> `
//
// The third row is what says which pass the option gates, and it is why this
// is interp.PromptStyle.Expand rather than a switch over the whole rendering.
//
// A pty was needed to measure *bash* and is needed nowhere here: the rendering
// is a function, so this file calls it. A pty test would put the thing being
// measured inside a terminal session and a line editor, which is where a
// one-line PS1 hides a composition bug in this tree.
//
// PS4 is not this option's business — `shopt -u promptvars; PS4='+$x+'; set
// -x` still expands, measured the same day — so there is no non-interactive
// route to the difference in bash either, and none is claimed here (#4149).

// drawnPrompt renders text the way the front end draws PS1, after running
// setup on the shell.
func drawnPrompt(t *testing.T, setup, text string) string {
	t.Helper()
	var buf strings.Builder
	r := preset.Runner(dialecttest.Base{
		Name: "sh", Stdout: &buf, Stderr: &buf, Dir: t.TempDir(),
		Env: []string{"PATH=/usr/bin:/bin"},
	})
	if setup != "" {
		if _, err := r.Run(t.Context(), preset.Parse(t, setup)); err != nil {
			t.Fatalf("setup %q: %v", setup, err)
		}
	}
	out, _, ok := interp.RenderPromptValue(bash.PromptStyle(), r, text, r.PromptField, nil)
	if !ok {
		t.Fatalf("rendering %q was refused", text)
	}
	return out
}

// TestThePromptIsExpandedUnlessTheOptionIsOff is the pair. The "on" half alone
// would pass against a shell that expanded unconditionally, which is what this
// one did.
func TestThePromptIsExpandedUnlessTheOptionIsOff(t *testing.T) {
	for _, tc := range []struct{ setup, text, want, why string }{
		{`x=XVAL`, `[$x]> `, `[XVAL]> `, "a parameter, option on"},
		{`shopt -u promptvars; x=XVAL`, `[$x]> `, `[$x]> `, "a parameter, option off"},
		{``, `[$(echo SUB)]> `, `[SUB]> `, "a command substitution, option on"},
		{`shopt -u promptvars`, `[$(echo SUB)]> `, `[$(echo SUB)]> `, "a command substitution, option off"},
		{`x=1`, `[$((x+1))]> `, `[2]> `, "arithmetic, option on"},
		{`shopt -u promptvars; x=1`, `[$((x+1))]> `, `[$((x+1))]> `, "arithmetic, option off"},
		// And it can be turned back on, which is what says the answer is read
		// at every draw rather than settled once.
		{`shopt -u promptvars; shopt -s promptvars; x=XVAL`, `[$x]> `, `[XVAL]> `, "off then on again"},
	} {
		if got := drawnPrompt(t, tc.setup, tc.text); got != tc.want {
			t.Errorf("%s: drew %q, want %q", tc.why, got, tc.want)
		}
	}
}

// TestTheBackslashLanguageIsNotTheOptionsBusiness is the row that pins *which*
// pass the option gates. Measured: `PS1='[\u]> '` draws the user name with the
// option off. A switch over the whole rendering would pass every assertion
// above and fail this one.
func TestTheBackslashLanguageIsNotTheOptionsBusiness(t *testing.T) {
	on := drawnPrompt(t, ``, `[\u]> `)
	off := drawnPrompt(t, `shopt -u promptvars`, `[\u]> `)
	if on != off {
		t.Errorf("the escapes moved with the option: on %q, off %q", on, off)
	}
	if on == `[\u]> ` {
		t.Errorf("the escape was not drawn at all: %q — the row cannot discriminate if nothing expands it", on)
	}
}

// TestThePromptvarsNameIsListedAndMoves: the listing and both statuses. A
// refusal at 1 on `shopt -u` is the state this replaces.
func TestThePromptvarsNameIsListedAndMoves(t *testing.T) {
	out, st := answersRun(t, `shopt promptvars
shopt -u promptvars; echo "u=$?"
shopt promptvars
shopt -s promptvars; echo "s=$?"
shopt promptvars`)
	want := "promptvars          \ton\nu=0\npromptvars          \toff\ns=0\npromptvars          \ton\n"
	if out != want || st != 0 {
		t.Errorf("status %d, output %q; want status 0 and %q", st, out, want)
	}
}
