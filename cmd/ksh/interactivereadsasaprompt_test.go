// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// An interactive ksh93 reads a `-c` string and a script file the way it reads
// a prompt: each input — a logical line, with whatever it takes to finish it —
// restarts `$LINENO` at 1, a run-time failure is located from it, and a parse
// failure is worded with no line (#6597).
//
// The noun is the shell being interactive, not the route. The deciding pairs
// hold the text fixed: the same program under `-c` and `-i -c`, and the same
// file with and without `-i`, part company, while `-i -c`, `-i t.sh` and `-i`
// with the program on a pipe all agree. Measured 2026-10-15 on ksh93u+
// 2012-08-01 with `ENV=/dev/null`; every expected line is that shell's, the
// file's name in place of `ksh` on the file route, and under `-i` it ends
// with the newline the end of the input writes (#6654, see
// TestAnInteractiveKshEndsTheLineAtTheEndOfItsProgram). This shell numbered the
// string and the file whole, as without `-i`: `L=5`, `L2=7`, `FL=11`,
// `ksh[14]: cd: …` and `syntax error at line 3`.
func TestAnInteractiveKshReadsItsProgramAsAPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	t.Setenv("HISTFILE", filepath.Join(home, ".sh_history"))
	t.Chdir(home)
	const program = ":\n:\nif true; then\n:\nnosuch_zq; echo L=$LINENO\nfi\necho L2=$LINENO\n" +
		"f(){\n:\nnosuch_zr\necho FL=$LINENO\n}\nf\ncd /nonexist_zq\n" +
		"eval \"true\nnosuch_ze\"\n"
	const runOut = "L=3\nL2=1\nFL=4\n"
	runErrs := func(name string) string {
		return name + ": line 3: nosuch_zq: not found\n" +
			name + ": line 3: nosuch_zr: not found\n" +
			name + ": cd: /nonexist_zq: [No such file or directory]\n" +
			name + ": eval: line 2: nosuch_ze: not found\n\n"
	}
	const broken = "echo a\nif true\nthen )\nfi\necho b\n"
	brokenErrs := func(name string) string { return name + ": syntax error: `)' unexpected\n\n" }
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(home, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("t.sh", program)
	write("b.sh", broken)
	for _, c := range []struct {
		name            string
		argv            []string
		wantOut, wantEr string
		wantCode        int
	}{
		{"-i -c", []string{"ksh", "-i", "-c", program}, runOut, runErrs("ksh"), 0},
		{"-i script", []string{"ksh", "-i", "t.sh"}, runOut, runErrs("t.sh"), 0},
		{"-i -c, a parse failure", []string{"ksh", "-i", "-c", broken}, "a\n", brokenErrs("ksh"), 3},
		{"-i script, a parse failure", []string{"ksh", "-i", "b.sh"}, "a\n", brokenErrs("b.sh"), 3},
		// A substitution's body is refused as the line runs, and says so as
		// it would at a prompt.
		{"-i -c, a refused substitution body", []string{"ksh", "-i", "-c", "echo $(fi)"}, "", "ksh: syntax error: `fi' unexpected\n\n", 3},
		// The other half of each pair: the same text, not interactive.
		{
			"-c",
			[]string{"ksh", "-c", program},
			"L=5\nL2=7\nFL=11\n",
			"ksh: line 5: nosuch_zq: not found\nksh: line 10: nosuch_zr: not found\n" +
				"ksh[14]: cd: /nonexist_zq: [No such file or directory]\nksh[15]: eval: line 2: nosuch_ze: not found\n", 0,
		},
		{
			"script",
			[]string{"ksh", "t.sh"},
			"L=5\nL2=7\nFL=11\n",
			"t.sh: line 5: nosuch_zq: not found\nt.sh: line 10: nosuch_zr: not found\n" +
				"t.sh[14]: cd: /nonexist_zq: [No such file or directory]\nt.sh[15]: eval: line 2: nosuch_ze: not found\n", 0,
		},
		{"-c, a parse failure", []string{"ksh", "-c", broken}, "a\n", "ksh: syntax error at line 3: `)' unexpected\n", 3},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := prompt(t, "", c.argv...)
			// The status of the last command is not what is asked here, so
			// only the parse failure's is held.
			if out != c.wantOut || errs != c.wantEr || (c.wantCode != 0 && code != c.wantCode) {
				t.Errorf("got (%q, %q, %d), want (%q, %q, %d)", out, errs, code, c.wantOut, c.wantEr, c.wantCode)
			}
		})
	}
}
