// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A `<&` operand is read while the redirection is **set up**, not while the
// word is read, so a parameter holding a descriptor is taken here exactly as
// it is in the other four columns.
//
// This used to be a grammar flag, and the measurement that put it there was
// taken with the parameters **unset** — under `zsh -n`, where a command
// substitution does not run either — so every row read as a refusal because
// every row expanded to nothing. Re-measured 2026-09-19 on zsh 5.9.2 under
// `-f`, script files under `env -i PATH=/usr/bin:/bin LC_ALL=C` with `v=5`
// and fd 5 open, the rule is one line: **expand the word, and what it comes
// to has to be a non-empty run of digits**, or the `-` that closes the
// descriptor, or the coprocess `p`.
//
//	head -1 <&$v   <&"$v"   <&${v}   <&$(echo 5)   <&`echo 5`   <&""$v   runs
//	head -1 <&5$v  <&${v}5  <&$(echo 5)5                 55: bad file descriptor
//	head -1 <&x    <&5x     <&5$(echo q)   <&$v5   <&x$v   file number expected
//
// The middle row is the one that says the check is on the *expansion* and not
// on the literal text the parser holds: `5$v` comes to `55`, which is a file
// number and simply is not open. `$v5` is the parameter `v5`, unset, so it
// comes to nothing and is refused — the same rule, not a second one (#3826).
func TestAnInputDuplicatesOperandIsReadWhenTheRedirectionIsSetUp(t *testing.T) {
	d := zsh.Dialect()
	for _, src := range []string{
		"cat <&$v", `cat <&"$v"`, "cat <&${v}", "cat <&$(echo 5)", "cat <&x",
		"cat <&5x", "exec {fd}<&$v", "cat <&5", "cat <&-", "cat <&p",
		"cat >&$v", "cat 2>&$v",
	} {
		if _, err := syntax.Parse(src, d.On(syntax.RouteFromScriptFile)); err != nil {
			t.Errorf("parse %q: %v — this shell reads the word and answers at run time", src, err)
		}
	}
}

// The answers themselves, against the real shell's. The refusal is worth a
// row of its own because a parser that simply stopped refusing would also
// pass the rows above while letting `<&x` open a file called `x`.
func TestAnInputDuplicateFromAParameterDuplicates(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("line-one\nline-two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			name:   "a parameter holding a descriptor",
			src:    "exec 5< f; v=5; IFS= read -r l <&$v; print -r -- \"[$l]\"",
			want:   "[line-one]\n",
			status: 0,
		},
		{
			// The control that says the parameter is what is being read: the
			// same line with the number written out.
			name:   "the number written out",
			src:    "exec 5< f; IFS= read -r l <&5; print -r -- \"[$l]\"",
			want:   "[line-one]\n",
			status: 0,
		},
		{
			name:   "a substitution holding a descriptor",
			src:    "exec 5< f; IFS= read -r l <&$(echo 5); print -r -- \"[$l]\"",
			want:   "[line-one]\n",
			status: 0,
		},
		{
			// `5$v` with v=5 is `55`, which is a file number and is not open
			// — a different sentence from the refusal below, which is what
			// says the expansion was read as a number at all.
			name:   "a number that is not open",
			src:    "exec 5< f; v=5; IFS= read -r l <&5$v",
			want:   "55: bad file descriptor",
			status: 1,
		},
		{
			name:   "a word that is not a number",
			src:    "exec 5< f; IFS= read -r l <&x",
			want:   "file number expected",
			status: 1,
		},
		{
			// An unset parameter comes to nothing, which is not a run of
			// digits. This is the row the old grammar flag was measured on,
			// and it is the only one it got right.
			name:   "an unset parameter",
			src:    "exec 5< f; IFS= read -r l <&$nope",
			want:   "file number expected",
			status: 1,
		},
		{
			// The line before it runs, which is what says this is a run-time
			// refusal rather than a parse one — the file was read whole and
			// only the redirection failed. The line *after* it does not run,
			// and that is not a second bug: zsh ends the shell for this on a
			// **builtin** and carries on for an external command, which is
			// DuplicationTargetErrorEndsTheShellOnABuiltin. Measured
			// 2026-09-19 on zsh 5.9.2: the same three lines with `cat <&x` in
			// the middle print `one`, the sentence, and `two`, at 0.
			name:   "a refusal on a builtin ends the shell, after the line before it ran",
			src:    "print -r -- one\nIFS= read -r l <&x\nprint -r -- two\n",
			want:   "one\n",
			status: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("output = %q, want %q in it", out, tc.want)
			}
			if strings.HasPrefix(tc.name, "a refusal on a builtin") && strings.Contains(out, "two") {
				t.Errorf("output = %q, want the shell to have ended at the refusal", out)
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
		})
	}
}
