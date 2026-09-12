// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A name that is another preset's builtin is named as one, and a typo is not.
func TestTheHintNamesThePresetsThatHaveTheName(t *testing.T) {
	for _, tc := range []struct{ name, except, want string }{
		{"whence", "", "`whence` is a builtin of -dialect ksh and -dialect zsh"},
		{"shopt", "", "`shopt` is a builtin of -dialect bash"},
		// A typo is a typo, which is the half that keeps this off the
		// common case.
		{"lss", "", ""},
		{"zzznope", "", ""},
		// The preset the shell is already in is left out, so the hint is
		// never advice to switch to where you are.
		{"whence", "zsh", "`whence` is a builtin of -dialect ksh"},
		{"shopt", "bash", ""},
	} {
		got := hintFor(tc.name, tc.except)
		if tc.want == "" {
			if got != "" {
				t.Errorf("hintFor(%q, %q) = %q, want nothing", tc.name, tc.except, got)
			}
			continue
		}
		if !strings.Contains(got, tc.want) {
			t.Errorf("hintFor(%q, %q) = %q, want it to carry %q", tc.name, tc.except, got, tc.want)
		}
		if !strings.HasPrefix(got, "      ") {
			t.Errorf("hintFor(%q, %q) = %q, want it indented under the first line", tc.name, tc.except, got)
		}
	}
}

// The sentence lists one, two and three the way a sentence does.
func TestTheHintReadsAsASentence(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want string
	}{
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a and b"},
		{[]string{"a", "b", "c"}, "a, b and c"},
	} {
		if got := joinAnd(tc.in); got != tc.want {
			t.Errorf("joinAnd(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The wiring reaches the runner, and it chains whatever registration the
// preset already had rather than replacing it. Both halves, because a hint
// that silently dropped a dialect's builtins would leave a shell that looks
// like this one and cannot run `whence`.
func TestWithDialectHintChainsTheRegistration(t *testing.T) {
	reached := false
	sh := withDialectHint(driver.Shell{Register: func(*interp.Runner) { reached = true }}, "core")
	r := &interp.Runner{}
	sh.Register(r)
	if !reached {
		t.Error("the preset's own registration was replaced rather than chained")
	}
	if r.NotFoundHint == nil {
		t.Fatal("the hint did not reach the runner")
	}
	// A preset with no registration of its own still gets one.
	r2 := &interp.Runner{}
	withDialectHint(driver.Shell{}, "core").Register(r2)
	if r2.NotFoundHint == nil {
		t.Fatal("a preset with no Register of its own got no hint")
	}
}

// And it writes nothing outside a prompt, which is the gate that keeps this
// out of every script's standard error. No real shell writes a second line
// here, so a script must see exactly what it saw before.
func TestTheHintIsWrittenOnlyAtAPrompt(t *testing.T) {
	r := &interp.Runner{}
	withDialectHint(driver.Shell{}, "core").Register(r)
	if got := r.NotFoundHint("whence"); got != "" {
		t.Errorf("a script got the hint %q, want nothing", got)
	}
	r.AtPrompt = true
	if got := r.NotFoundHint("whence"); got == "" {
		t.Error("a prompt got no hint")
	}
}

// End to end through the interpreter: the first line is byte-identical with
// the hint and without it, and the hint is the second.
//
// The first half is the load-bearing one. A script parsing the diagnostic
// sees what it saw before, which is what makes this an addition rather than a
// change to what the shell says.
func TestTheFirstLineIsUnchanged(t *testing.T) {
	line := func(hint bool) (string, string) {
		t.Helper()
		var out bytes.Buffer
		// A PATH, because an empty one is an axis this dialect has not
		// answered and the shell says so before it says anything else.
		r := &interp.Runner{
			Stdout: &out, Stderr: &out, AtPrompt: true,
			Env: []string{"PATH=/usr/bin:/bin"},
		}
		if hint {
			withDialectHint(driver.Shell{}, "core").Register(r)
		}
		f, err := syntax.Parse("whence ls\n", syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Run(t.Context(), f); err != nil {
			t.Fatal(err)
		}
		lines := strings.SplitN(strings.TrimRight(out.String(), "\n"), "\n", 2)
		if len(lines) == 1 {
			return lines[0], ""
		}
		return lines[0], lines[1]
	}
	plain, plainRest := line(false)
	hinted, hintedRest := line(true)
	if plain != hinted {
		t.Errorf("the first line moved:\n without %q\n with    %q", plain, hinted)
	}
	if plainRest != "" {
		t.Errorf("a shell with no hint wrote a second line %q", plainRest)
	}
	if !strings.Contains(hintedRest, "-dialect ksh") {
		t.Errorf("the second line is %q, want the presets that have the name", hintedRest)
	}
}
