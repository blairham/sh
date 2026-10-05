// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// In an interactive bash a parse failure in `eval`'s text is worded as one in
// a typed line where no file is being read — the sentence alone after the
// shell's name — and as a script's `eval` is, with the file's line and the
// text quoted back, where one is (#6056). See
// interp.Diagnostics.InteractiveShellSpeaksAsAtAPrompt.
//
// Measured 2026-10-05 on bash 5.3.20 (/opt/homebrew/bin/bash) through a
// pseudo-terminal and on a pipe alike, `env -i` with a scratch HOME; every
// expected stderr is that shell's. This shell wrote `bash: eval: …` and
// quoted the text back everywhere, with no line.
func TestAnEvalParseFailureAtAPromptIsWordedAsATypedLine(t *testing.T) {
	const near = "syntax error near unexpected token `)'\n"
	for _, c := range []struct {
		name, typed string
		argv        []string
		want        string
	}{
		{"typed", "eval \"echo )\"\n", []string{"bash", "--norc", "-i"}, "bash: " + near},
		{"-i -c", "", []string{"bash", "--norc", "-i", "-c", "eval \"echo )\""}, "bash: " + near},
		{"a function a file defined", ". ./lib\nk\n", []string{"bash", "--norc", "-i"}, "bash: " + near},
		{
			"the line in the sentence moves with the prompt's", "echo a\neval \"echo x\nif\"\n",
			[]string{"bash", "--norc", "-i"},
			"bash: syntax error: unexpected end of file from `if' command on line 3\n",
		},
		{
			"a file . read", ". ./g\n",
			[]string{"bash", "--norc", "-i"},
			"bash: eval: line 2: " + near + "bash: eval: line 2: `echo )'\n",
		},
		{
			"an -i script", "",
			[]string{"bash", "--norc", "-i", "g"},
			"bash: eval: line 2: " + near + "bash: eval: line 2: `echo )'\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := scratchHome(t)
			t.Chdir(home)
			writeHomeFile(t, home, "g", "true\neval \"echo )\"\n")
			writeHomeFile(t, home, "lib", "k(){ eval \"echo )\"; }\n")
			_, errs, _ := prompt(t, c.typed, c.argv...)
			// The diagnostics alone: not the prompts and the echo of what was
			// typed, nor the two lines about job control a shell with no
			// terminal writes first.
			var got strings.Builder
			for _, l := range strings.SplitAfter(errs, "\n") {
				if strings.Contains(l, "syntax error") || strings.Contains(l, "`echo )'") {
					got.WriteString(l)
				}
			}
			if got.String() != c.want {
				t.Errorf("diagnostics %q, want %q (stderr %q)", got.String(), c.want, errs)
			}
		})
	}
}
