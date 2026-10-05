// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// An interactive bash names every diagnostic the way it names a line typed at
// its prompt — the last component of the name it was started as, and no line
// — whatever text it came from; a file's *parse* failure keeps the file and
// its line, with that name in front (#6008). See
// interp.Diagnostics.InteractiveShellSpeaksAsAtAPrompt.
//
// Measured 2026-10-05 on bash 5.3.20 (/opt/homebrew/bin/bash) through a
// pseudo-terminal and on a pipe alike, `env -i` with a scratch HOME; every
// expected line is that shell's, with the home directory standing for @. This
// shell named the files, with no line, and wrote the whole of `$0`.
func TestAnInteractiveBashSpeaksAsAtItsPrompt(t *testing.T) {
	for _, c := range []struct {
		name, typed string
		argv        []string
		want        []string
	}{
		{"the prompt", ". ./g\nf(){ nosuch3; }; f\n", []string{"/opt/homebrew/bin/bash", "-i"}, []string{
			"bash: nosuchRC: command not found\n",
			"bash: nosuch: command not found\n",
			"bash: unset: boom\n",
			"bash: nosuch3: command not found\n",
			"bash: ./bad: line 1: syntax error near unexpected token `)'\n",
			"bash: ./bad: line 1: `echo )'\n",
		}},
		{"-i -c", "", []string{"/opt/homebrew/bin/bash", "-i", "-c", "nosuchC; . ./g"}, []string{
			"bash: nosuchRC: command not found\n",
			"bash: nosuchC: command not found\n",
			"bash: nosuch: command not found\n",
		}},
		{"a name of its own", "nosuchA\n", []string{"./weird/mybash", "-i"}, []string{
			"mybash: nosuchRC: command not found\n",
			"mybash: nosuchA: command not found\n",
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := scratchHome(t)
			t.Chdir(home)
			writeHomeFile(t, home, ".bashrc", "true\nnosuchRC\n")
			writeHomeFile(t, home, "g", "true\nnosuch\necho ${unset?boom}\n")
			writeHomeFile(t, home, "bad", "echo )\n")
			typed := c.typed
			if c.name == "the prompt" {
				typed += ". ./bad\n"
			}
			_, errs, _ := prompt(t, typed, c.argv...)
			for _, want := range c.want {
				if !strings.Contains(errs, strings.ReplaceAll(want, "@", home)) {
					t.Errorf("stderr %q, want a line %q", errs, want)
				}
			}
			if strings.Contains(errs, home) || strings.Contains(errs, "line 2:") {
				t.Errorf("stderr %q names a file or a line it should not", errs)
			}
		})
	}
}

// And a startup file's parse failure under `-i` keeps its path and line, with
// the name in front: `bash: @/.bashrc: line 1: syntax error near unexpected
// token `)'`. Without `-i` it is the file alone, as before.
func TestAnInteractiveBashrcParseFailureIsPrefixed(t *testing.T) {
	home := scratchHome(t)
	writeHomeFile(t, home, ".bashrc", "echo )\n")
	_, errs, _ := prompt(t, "", "bash", "-i", "-c", ":")
	for _, want := range []string{
		"bash: " + home + "/.bashrc: line 1: syntax error near unexpected token `)'\n",
		"bash: " + home + "/.bashrc: line 1: `echo )'\n",
	} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr %q, want a line %q", errs, want)
		}
	}
}
