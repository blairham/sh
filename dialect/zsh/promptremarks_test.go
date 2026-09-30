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

// **A line typed at the prompt has its parser remarks written** (#5239): a
// function named by an alias draws `defining function based on alias` ahead
// of its parse error there, exactly as in a script.
//
// The prompt wrote remarks only for what was left over at the end of the
// input, on the reasoning that the one remark then known was a here-document
// the input ran out inside. zsh's refusal is a remark about a *finished* line,
// so it was never said. Measured 2026-09-30 against zsh 5.9.2 under `-fi` with
// the lines on stdin.
//
// Asserted as an ordered pair and as a **count**. The first prompt carries the
// host's name before `PROMPT=”` takes effect, so the whole stream cannot be
// compared; and a remark written each time a growing construct is parsed again
// would still contain the pair, which only the count catches.
func TestAPromptWritesARemarkAboutAFinishedLine(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const prep = "unsetopt PROMPT_SP\nPROMPT=''\nPS2=''\n"
	remark := func(name string) string {
		return "zsh: defining function based on alias `" + name + "'\nzsh: parse error near `()'\n"
	}
	for _, c := range []struct {
		name, src, out string
		pairs          []string
	}{
		{"a regular alias", "alias ga=x\nga () { :; }\nprint DONE\n", "DONE\n", []string{remark("ga")}},
		{"a global alias", "alias -g GA=x\nGA () { :; }\nprint DONE\n", "DONE\n", []string{remark("GA")}},
		// One to each refused line, and not one to each time a line is read.
		{
			"two refused lines", "alias ga=x gb=y\nga () { :; }\ngb () { :; }\nprint DONE\n", "DONE\n",
			[]string{remark("ga"), remark("gb")},
		},
		{"a clean line", "print ok\n", "ok\n", nil},
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
			driver.MainArgs(sh, []string{"zsh", "-fi"})
			got := errs.String()
			if out.String() != c.out {
				t.Errorf("out %q, want %q", out.String(), c.out)
			}
			if n := strings.Count(got, "defining function based on alias"); n != len(c.pairs) {
				t.Errorf("%d remarks in %q, want %d", n, got, len(c.pairs))
			}
			rest := got
			for _, pair := range c.pairs {
				i := strings.Index(rest, pair)
				if i < 0 {
					t.Fatalf("%q not in %q, in order", pair, got)
				}
				rest = rest[i+len(pair):]
			}
		})
	}
}
