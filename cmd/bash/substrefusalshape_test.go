// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// A refused substitution body is worded as a parse failure of the text it is
// in, on every route an interactive bash has (#6279). See
// interp's Runner.interactiveSubstRefusal.
//
// Measured 2026-10-06 against /opt/homebrew/bin/bash 5.3.20, `env -i`,
// `--norc`, started as `bash`; every expected stderr is that shell's, and the
// last two rows are the non-interactive controls.
func TestARefusedSubstitutionBodyIsWordedAsTheTextItIsIn(t *testing.T) {
	const msg = "syntax error near unexpected token `fi' while looking for matching `)'"
	for _, c := range []struct {
		name, typed string
		argv        []string
		script      string
		want        string
	}{
		{
			"typed at the prompt", "echo $(fi)\n",
			[]string{"bash", "--norc", "-i"},
			"",
			"bash: " + msg + "\nbash: syntax error\n",
		},
		{
			"eval'd at the prompt", "eval \"echo \\$(fi)\"\n",
			[]string{"bash", "--norc", "-i"},
			"",
			"bash: " + msg + "\n",
		},
		{
			"under -i -c", "",
			[]string{"bash", "--norc", "-i", "-c", "echo $(fi)"},
			"",
			"bash: " + msg + "\n",
		},
		{
			"in an -i script", "",
			[]string{"bash", "--norc", "-i", "s.sh"},
			"echo $(fi)\n",
			"bash: s.sh: line 1: " + msg + "\nbash: s.sh: line 1: `echo $(fi)'\n",
		},
		{
			"eval'd in an -i script", "",
			[]string{"bash", "--norc", "-i", "s.sh"},
			"eval \"echo \\$(fi)\"\n",
			"bash: eval: line 1: " + msg + "\nbash: eval: line 1: `echo $(fi)'\n",
		},
		{
			"in a file . read under -i -c", "",
			[]string{"bash", "--norc", "-i", "-c", ". ./g"},
			"",
			"bash: ./g: line 2: " + msg + "\nbash: ./g: line 2: `echo $(fi)'\n",
		},
		{
			"backquoted under -i -c", "",
			[]string{"bash", "--norc", "-i", "-c", "echo `fi`"},
			"",
			"bash: command substitution: line 1: syntax error near unexpected token `fi'\n" +
				"bash: command substitution: line 1: `fi'\n",
		},
		{
			"a file . read under -c", "",
			[]string{"bash", "--norc", "-c", ". ./g"},
			"",
			"./g: line 2: " + msg + "\n./g: line 2: `echo $(fi)'\n",
		},
		{
			"a script", "",
			[]string{"bash", "--norc", "s.sh"},
			"echo $(fi)\n",
			"s.sh: line 1: " + msg + "\ns.sh: line 1: `echo $(fi)'\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := scratchHome(t)
			t.Chdir(home)
			writeHomeFile(t, home, "g", "true\necho $(fi)\n")
			if c.script != "" {
				writeHomeFile(t, home, "s.sh", c.script)
			}
			_, errs, _ := prompt(t, c.typed, c.argv...)
			var kept []string
			for _, line := range strings.SplitAfter(errs, "\n") {
				// The job-control notices, the prompts with the lines they
				// echo, and `exit` are not this test's; the refusal is every
				// other line that names it.
				if strings.HasPrefix(line, "bash-5.3$ ") {
					continue
				}
				if strings.Contains(line, "fi") || line == "bash: syntax error\n" {
					kept = append(kept, line)
				}
			}
			if got := strings.Join(kept, ""); got != c.want {
				t.Errorf("stderr reads\n%s\nwant\n%s(whole stderr %q)", got, c.want, errs)
			}
		})
	}
}
