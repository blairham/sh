// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// **An option that moves the grammar reaches the prompt** (#5241): set on one
// line, it decides how the next line typed is read, as it does in a script.
//
// The options that move the grammar move the runner's dialect, and the prompt
// read every line with the copy it was handed at startup — so none of them
// took effect there, whether set at the prompt or on the command line. The
// script route was already right, and these rows are the prompt's half of the
// ones that proved it. Measured 2026-09-30 against zsh 5.9.2 under `-fi`.
//
// **The two comment rows are why the comments rule is kept on top rather than
// dropped.** It is the prompt's own rule and never lived in the runner's
// dialect (#2563); read the runner's grammar and forget to reapply it, and a
// `#` typed with interactivecomments off would become a comment again. The
// combined row is the one that needs both halves at once.
func TestAGrammarOptionReachesThePrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const prep = "unsetopt PROMPT_SP\nPROMPT=''\nPS2=''\n"
	for _, c := range []struct {
		name string
		args []string
		src  string
		out  string
		err  string // a diagnostic that must appear; "" means none may
	}{
		{"aliasfuncdef", []string{"-fi"}, "alias ga=x\nsetopt aliasfuncdef\nga () { print ran; }\nx\n", "ran\n", ""},
		{"posixaliases", []string{"-fi"}, "alias '!'='print took'\nsetopt posixaliases\n! true\necho st=$?\n", "st=1\n", ""},
		{"nomultifuncdef", []string{"-fi"}, "setopt nomultifuncdef\naa bb () { :; }\nprint DONE\n", "DONE\n", "zsh: parse error near `()'\n"},
		{"noshortloops", []string{"-fi"}, "unsetopt shortloops\nfor i in a; print $i\nprint DONE\n", "DONE\n", "zsh: parse error near `print'\n"},
		{"ignorebraces", []string{"-fi"}, "setopt ignorebraces\n{ echo A }\nprint DONE\n", "", "zsh: parse error near `\\n'\n"},
		// Given on the command line, the option moves the runner's grammar
		// before the first prompt, and the prompt follows it from the start.
		{
			"posixaliases on the command line",
			[]string{"-f", "-o", "posixaliases", "-i"},
			"alias '!'='print took'\n! true\necho st=$?\n", "st=1\n", "",
		},
		{
			"a comment and a grammar option together",
			[]string{"-fi"},
			"setopt interactivecomments posixaliases\nalias '!'='print took'\n! true # c\necho st=$?\n", "st=1\n", "",
		},
		{
			"interactivecomments off still reads # as text",
			[]string{"-fi"},
			"unsetopt interactivecomments\necho a # b\n", "a # b\n", "",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "in.sh")
			if err := os.WriteFile(path, []byte(prep+c.src), 0o600); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			var out, errs bytes.Buffer
			sh := zshShell()
			sh.Stdin, sh.Stdout, sh.Stderr = f, &out, &errs
			driver.MainArgs(sh, append([]string{"zsh"}, c.args...))
			if out.String() != c.out {
				t.Errorf("out %q, want %q", out.String(), c.out)
			}
			got := errs.String()
			if c.err == "" {
				if strings.Contains(got, "zsh:") {
					t.Errorf("said %q, want no diagnostic", got)
				}
			} else if strings.Count(got, c.err) != 1 {
				t.Errorf("err %q, want %q once", got, c.err)
			}
		})
	}
}
