// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// One shell steps over a token standing where an `if` or `elif` clause's
// first command was due and refuses the one after it, and the other four
// name the token they met (#3961).
//
// Measured 2026-09-21 from a script file, `timeout 3 env -i
// PATH=/usr/bin:/bin LC_ALL=C <shell> -f s.sh` with standard input on the
// null device, in a fresh directory.
//
// **The second row is why this is a grammar flag and not a wording.** The
// parenthesis is *consumed*, so it is no longer there to close anything: the
// substitution runs to the end of the file in the shell that steps over it
// and ends at that parenthesis in the two that do not. Both readers inside
// this parser have to agree about it — the grammar's read of the body and
// the counting loop under it — which they do by construction, because the
// loop's bound is how far the read got. See syntax.Lexer.lastBodyStop.
//
// The control any fix has to pass is the `: post` line: it makes the
// refusal fall on the *next line's* token, so a change that matched the
// one-line shape by accident is caught here.
func TestAnIfClauseStepsOverWhatItCannotUse(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      map[string]string
	}{
		{
			name: "at the top level, with no substitution anywhere",
			src:  "printf 'start\\n'\nif true; then ) echo X; fi\n",
			want: map[string]string{
				"zsh": "s.sh:2: parse error near `echo'\n",
				"bash": "s.sh: line 2: syntax error near unexpected token `)'\n" +
					"s.sh: line 2: `if true; then ) echo X; fi'\n",
				"dash": "s.sh: 2: Syntax error: \")\" unexpected\n",
			},
		},
		{
			name: "and so the substitution it was inside never closes",
			src:  "printf 'start\\n'\nv=$(echo hi; if true; then)\n: post\n",
			want: map[string]string{
				"zsh": "s.sh:3: parse error near `:'\n" +
					"s.sh:4: parse error near `v=$(echo hi; if true...'\n",
				"bash": "s.sh: line 2: syntax error near unexpected token `)'\n" +
					"s.sh: line 2: `v=$(echo hi; if true; then)'\n",
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for name, want := range c.want {
				t.Run(name, func(t *testing.T) {
					dir := t.TempDir()
					if err := os.WriteFile(filepath.Join(dir, "s.sh"), []byte(c.src), 0o644); err != nil {
						t.Fatal(err)
					}
					t.Chdir(dir)
					sh := echoShells()[name]
					var out, errs strings.Builder
					sh.Stdout, sh.Stderr = &out, &errs
					driver.MainArgs(sh, []string{sh.Name, "s.sh"})
					if got := errs.String(); got != want {
						t.Errorf("wrote\n%s\nwant\n%s", got, want)
					}
					if !strings.HasPrefix(out.String(), "start\n") {
						t.Errorf("standard output was %q; the line before it has to have run", out.String())
					}
				})
			}
		})
	}
}
