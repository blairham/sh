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
				if !hasLine(errs, strings.ReplaceAll(want, "@", home)) {
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
		if !hasLine(errs, want) {
			t.Errorf("stderr %q, want a line %q", errs, want)
		}
	}
}

// A line typed at the prompt that will not parse is named the same way as the
// command that fails on the line after it, and so is the warning about a
// here-document the input ran out inside (#6244). The prompt's parse failures
// are worded by the front end, not by the Runner, and the front end was handed
// the name the shell was started as: `./weird/mybash: syntax error …` beside
// `mybash: nosuch: command not found`.
//
// Measured 2026-10-06 on bash 5.3.20 (/opt/homebrew/bin/bash) on a pipe,
// `env -i` with a scratch HOME, `--norc -i`, started as `./weird/mybash`:
// every expected line below is that shell's. Started as
// `/opt/homebrew/bin/bash` it writes `bash:`, and under `exec -a -bash`,
// `-bash:` — the last component of argv[0], a login's dash kept.
func TestAnInteractiveBashNamesALineThatWillNotParseAsItNamesTheRest(t *testing.T) {
	for _, c := range []struct {
		name, typed, argv0 string
		want               []string
	}{
		{"a token and the end of the input", "fi\nnosuch\nfor x in 1\n", "./weird/mybash", []string{
			"mybash: syntax error near unexpected token `fi'",
			"mybash: nosuch: command not found",
			"mybash: syntax error: unexpected end of file from `for' command on line 3",
		}},
		{"a here-document the input ended", "cat <<EOF\nhi\n", "./weird/mybash", []string{
			"mybash: warning: here-document at line 1 delimited by end-of-file (wanted `EOF')",
		}},
		{"started by its whole path", "fi\n", "/opt/homebrew/bin/bash", []string{
			"bash: syntax error near unexpected token `fi'",
		}},
		{"a login", "fi\n", "-bash", []string{
			"-bash: syntax error near unexpected token `fi'",
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := scratchHome(t)
			t.Chdir(home)
			_, errs, _ := prompt(t, c.typed, c.argv0, "--norc", "--noprofile", "-i")
			for _, want := range c.want {
				if !hasLine(errs, want) && !hasLine(strings.ReplaceAll(errs, "> ", ""), want) {
					t.Errorf("stderr %q, want a line %q", errs, want)
				}
			}
			if c.argv0 != "-bash" && strings.Contains(errs, c.argv0+":") {
				t.Errorf("stderr %q names the shell by the whole of %q", errs, c.argv0)
			}
		})
	}
}

// And a parse failure, or a remark, in the program an interactive bash was
// handed rather than typed: the `-c` string is worded as a line typed at the
// prompt, and a script keeps its location and its echo with the shell's name
// in front (#6262). See driver's source.parseDiagnostic.
//
// Measured 2026-10-06 on bash 5.3.20 (/opt/homebrew/bin/bash), `env -i`,
// started as `./weird/mybash --norc -i`; every expected line is that shell's.
// Without `-i` the same inputs are `./weird/mybash: -c: line 1: …` and
// `s.sh: line 2: …`, which TestANonInteractiveBashKeepsTheScriptShape holds.
func TestAnInteractiveBashWordsItsProgramsParseFailuresAsAtItsPrompt(t *testing.T) {
	for _, c := range []struct {
		name   string
		args   []string
		script string
		want   []string
		status int
	}{
		{"a token in -c", []string{"-c", "true\nfi"}, "", []string{
			"mybash: syntax error near unexpected token `fi'",
		}, 2},
		{"the end of -c", []string{"-c", "echo a\nfor x in 1"}, "", []string{
			"mybash: syntax error: unexpected end of file from `for' command on line 2",
		}, 2},
		{"a here-document -c ended", []string{"-c", "cat <<EOF\nhi"}, "", []string{
			"mybash: warning: here-document at line 1 delimited by end-of-file (wanted `EOF')",
		}, 0},
		{"a token in a script", []string{"s.sh"}, "true\nfi\n", []string{
			"mybash: s.sh: line 2: syntax error near unexpected token `fi'",
			"mybash: s.sh: line 2: `fi'",
		}, 2},
		{"the end of a script", []string{"s.sh"}, "echo a\nfor x in 1\n", []string{
			"mybash: s.sh: line 3: syntax error: unexpected end of file from `for' command on line 2",
		}, 2},
		{"a here-document a script ended", []string{"s.sh"}, "cat <<EOF\nhi\n", []string{
			"mybash: warning: here-document at line 1 delimited by end-of-file (wanted `EOF')",
		}, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := scratchHome(t)
			t.Chdir(home)
			if c.script != "" {
				writeHomeFile(t, home, "s.sh", c.script)
			}
			argv := append([]string{"./weird/mybash", "--norc", "-i"}, c.args...)
			_, errs, code := prompt(t, "", argv...)
			for _, want := range c.want {
				if !hasLine(errs, want) {
					t.Errorf("stderr %q, want a line %q", errs, want)
				}
			}
			if strings.Contains(errs, "-c:") || strings.Contains(errs, "./weird/mybash:") {
				t.Errorf("stderr %q is in the non-interactive shape", errs)
			}
			if code != c.status {
				t.Errorf("status %d, want %d", code, c.status)
			}
		})
	}
}

// The other side: without `-i` nothing above changes.
func TestANonInteractiveBashKeepsTheScriptShape(t *testing.T) {
	home := scratchHome(t)
	t.Chdir(home)
	writeHomeFile(t, home, "s.sh", "true\nfi\n")
	_, errs, _ := prompt(t, "", "./weird/mybash", "--norc", "-c", "fi")
	if !hasLine(errs, "./weird/mybash: -c: line 1: syntax error near unexpected token `fi'") {
		t.Errorf("-c: stderr %q", errs)
	}
	_, errs, _ = prompt(t, "", "./weird/mybash", "--norc", "s.sh")
	if !hasLine(errs, "s.sh: line 2: syntax error near unexpected token `fi'") {
		t.Errorf("script: stderr %q", errs)
	}
}

// hasLine is whether text holds want as a whole line, prompt and all trimmed
// from its front: a substring check passes `./weird/mybash: …` for
// `mybash: …`, which is the very difference these tests are about.
func hasLine(text, want string) bool {
	want = strings.TrimSuffix(want, "\n")
	for _, line := range strings.Split(text, "\n") {
		if i := strings.LastIndex(line, "$ "); i >= 0 && strings.Contains(line[:i+2], "-5.") {
			line = line[i+2:]
		}
		if line == want {
			return true
		}
	}
	return false
}
