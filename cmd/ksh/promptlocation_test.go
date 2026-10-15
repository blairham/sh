// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// At a prompt ksh93 numbers each typed input from its own first line, and
// locates `eval`'s text by its own line as `ksh -c` does (#6074). See
// interp.Semantics.PromptNumbersEachInputFromOne and
// interp.Diagnostics.EvalTextAtAPromptKeepsItsLocation.
//
// Measured 2026-10-05 on ksh93u+ (/bin/ksh -i) through a pseudo-terminal and
// on a pipe alike, f holding `(exit 3)` and `echo ${unset?boom}`; every
// expected line is that shell's. This shell counted the session's lines —
// `L=2`, `ksh[6]: h[5]: …`, `ksh[7]: .: …` — and wrote `eval:` with no line.
func TestAKshPromptNumbersEachInputFromOne(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	// An interactive ksh writes its history, so the file is the test's own.
	t.Setenv("HISTFILE", filepath.Join(home, ".sh_history"))
	t.Chdir(home)
	if err := os.WriteFile(filepath.Join(home, "f"), []byte("(exit 3)\necho ${unset?boom}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	typed := "echo L=$LINENO\necho L=$LINENO\n" +
		"function h {\ntrue\n. ./f; }\nh\n" +
		"g(){ . ./f; }; g\n" +
		"eval \"true\necho \\${unset?boom}\"\n" +
		"function e2 { eval \"echo \\${unset?boom}\"; }; e2\n" +
		"eval \"echo )\"\n"
	out, errs, _ := prompt(t, typed, "ksh", "-i")
	if out != "L=1\nL=1\n" {
		t.Errorf("stdout %q, want L=1 twice", out)
	}
	for _, want := range []string{
		"ksh: h[3]: .: line 2: unset: boom\n",
		"$ ksh: .: line 2: unset: boom\n",
		"ksh: eval: line 2: unset: boom\n",
		"ksh: e2[1]: eval: line 1: unset: boom\n",
		// The parse half stays the prompt's.
		"ksh: eval: syntax error: `)' unexpected\n",
	} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr %q, want %q in it", errs, want)
		}
	}
}

// At a prompt a run-time failure past the first line of what was typed is
// located `line N:`, counted from that input's own first line, and one on the
// first line is not (#6581). The noun is the *line within the typed input*,
// not the function: the pair that says so is two top-level commands, one
// written on one line and one written over three, with no function in either.
//
// Measured 2026-10-15 on ksh93u+ 2012-08-01 under `-i` on a pipe; every
// expected line is that shell's. This shell located every one of them by name
// alone, as it does a parse failure there — which is the one kind that keeps
// no line at a prompt, because its sentence carried it.
func TestAKshPromptLocatesAFailurePastTheInputsFirstLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	t.Setenv("HISTFILE", filepath.Join(home, ".sh_history"))
	t.Chdir(home)
	for _, c := range []struct{ name, typed, want string }{
		{"one line, top level", "PS1=''\nnosuch_zq; :\n", "ksh: nosuch_zq: not found\n"},
		{"third line of a typed if", "PS1=''\nif true; then\n:\nnosuch_zq\nfi\n", "ksh: line 3: nosuch_zq: not found\n"},
		{"second line of a brace group", "PS1=''\n{ :\nnosuch_zq; }\n", "ksh: line 2: nosuch_zq: not found\n"},
		{"function body, from its definition", "PS1=''\nf(){\n:\n: $((1/0))\n}\nf\n", "ksh: line 3: 1/0: divide by zero\n"},
		{"function body on one line", "PS1=''\nf(){ nosuch_zq; }\nf\n", "ksh: nosuch_zq: not found\n"},
		{"keyword function", "PS1=''\nfunction k {\n:\nnosuch_zq\n}\nk\n", "ksh: k: line 3: nosuch_zq: not found\n"},
		{"a parse failure keeps no line", "PS1=''\nif true; then\n:\nfi )\n", "ksh: syntax error: `)' unexpected\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, errs, _ := prompt(t, c.typed, "ksh", "-i")
			// The prompts are written to the same stream, so the line is
			// looked for rather than compared whole; and a row with no line
			// must have none anywhere, which is the half a contains alone
			// would pass for the wrong reason.
			if !strings.Contains(errs, c.want) {
				t.Errorf("stderr %q, want %q in it", errs, c.want)
			}
			if !strings.Contains(c.want, "line ") && strings.Contains(errs, "line ") {
				t.Errorf("stderr %q names a line, want none", errs)
			}
		})
	}
}
