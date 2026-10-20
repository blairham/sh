// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tildeFlagDir is the tree these rows were measured against: a home holding
// `zz`, and three files a pattern matches beside one it does not.
//
// Built under t.TempDir() and never in a real home: the construct is about
// filename generation, so the rows only mean anything against a directory the
// test owns.
func tildeFlagDir(t *testing.T) (home, dir string) {
	t.Helper()
	root := t.TempDir()
	home = filepath.Join(root, "home")
	dir = filepath.Join(root, "d")
	for _, d := range []string{filepath.Join(home, "zz"), dir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"inner.sh", "inner2.sh", "other.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return home, dir
}

// `${~spec}` in this dialect, which is where the axes it overrides live.
//
// The substrate's tests name the grammar flag; this one names the shell,
// because the flag is only ever *visible* against this dialect's answers: an
// unquoted expansion's result is not a pattern here and a tilde in a value
// does not expand, so both halves are a no-op in the shells that read the
// characters as text. Measured 2026-09-06 on zsh 5.9.2.
func TestTheTildeFlagIsThisDialects(t *testing.T) {
	home, dir := tildeFlagDir(t)
	for _, tc := range []struct{ src, want string }{
		// The pattern half, against the dialect's own no.
		{`g="$D/inn*.sh"; printf "[%s]" ${g}`, `[DIR/inn*.sh]`},
		{`g="$D/inn*.sh"; printf "[%s]" ${~g}`, `[DIR/inner.sh][DIR/inner2.sh]`},
		{`g="$D/inn*.sh"; printf "[%s]" "${~g}"`, `[DIR/inn*.sh]`},
		{`g="$D/inn*.sh"; printf "[%s]" ${~~g}`, `[DIR/inn*.sh]`},
		// The tilde half, against a value no shell expands by default.
		{`t='~/zz'; printf "[%s]" ${t}`, `[~/zz]`},
		{`t='~/zz'; printf "[%s]" ${~t}`, `[HOME/zz]`},
		{`t='~/zz'; printf "[%s]" ${~~t}`, `[~/zz]`},
		// The shape zi.zsh writes eighteen times in nineteen lines, and the
		// reason a refusal here is worse than noise: the assignment has to
		// come back with the value, not empty.
		{`typeset -A Z; Z[H]="$HOME/.zi"; Z[H]=${~Z[H]}; printf "[%s]" "${Z[H]}"`, `[HOME/.zi]`},
		{`typeset -A Z; Z[H]='~/.zi'; Z[H]=${~Z[H]}; printf "[%s]" "${Z[H]}"`, `[HOME/.zi]`},
		{`P='~/zz'; P=${~P}; printf "[%s]" "$P"`, `[HOME/zz]`},
	} {
		src := "D=" + dir + "\nHOME=" + home + "\n" + tc.src
		out, st := runZsh(t, dir, src)
		got := strings.ReplaceAll(strings.ReplaceAll(out, dir, "DIR"), home, "HOME")
		if got != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, st, tc.want)
		}
	}
}

// A parse error is not what this is: the tilde is grammar here, so a `${~x}`
// in a file this dialect reads goes through.
func TestTheTildeFlagParsesInThisDialect(t *testing.T) {
	if _, err := parseZsh("echo ${~x}\necho ${~~x}\necho ${(U)~x}"); err != nil {
		t.Errorf("parse: %v", err)
	}
	// And the two shapes the shell refuses stay refused, as a bad
	// substitution when reached rather than at the read.
	home, dir := tildeFlagDir(t)
	for _, src := range []string{`echo ${~(U)x}`, `echo ${#~x}`} {
		out, st := runZsh(t, dir, "HOME="+home+"\n"+src)
		if !strings.Contains(out, "bad substitution") || st == 0 {
			t.Errorf("%s = %q (status %d), want a bad substitution", src, out, st)
		}
	}
}
