// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What a failing `cd` names when there was no operand to name.
//
// `cd` wrote its diagnostic against the first operand, so a bare `cd` reached
// into an empty slice and every dialect died of it — `internal error: runtime
// error: index out of range [0] with length 0`, which is not a diagnostic a
// script can act on and takes the whole shell with it. The subject is the
// place `cd` was *asked for*: HOME's value with no operand, OLDPWD's for
// `cd -`, the word as typed otherwise (#1483).
//
// Measured 2026-09-08 with `HOME=/no-such-home-1483 <shell> -c 'cd; echo st=$?'`:
//
//	bash 5.3.15  bash: line 1: cd: /no-such-home-1483: No such file or directory   1
//	bash-as-sh   sh: line 1: cd: /no-such-home-1483: No such file or directory     1
//	bash 3.2.57  bash: line 0: cd: /no-such-home-1483: No such file or directory   1
//	dash         dash: 1: cd: can't cd to /no-such-home-1483                       2
//	ksh93        ksh: cd: /no-such-home-1483: [No such file or directory]          1
//	zsh 5.9.2    zsh:cd:1: no such file or directory: /no-such-home-1483           1
//
// Whole lines, because the assertion that matters is which *name* is in the
// sentence and a substring over the reason would pass against a shell that
// named the word `cd`, named nothing, or printed the wrong path — and an
// assertion that only says "did not panic" passes against one that silently
// does nothing at all.
func TestEachDialectNamesHomeWhenABareCdCannotMove(t *testing.T) {
	const home = "/no-such-home-1483"
	for _, c := range []struct {
		dialect string
		want    string
	}{
		{"bash", "bash: line 1: cd: " + home + ": No such file or directory\nst=1\n"},
		{"dash", "dash: 1: cd: can't cd to " + home + "\nst=2\n"},
		{"ksh", "ksh: cd: " + home + ": [No such file or directory]\nst=1\n"},
		{"zsh", "zsh:cd:1: no such file or directory: " + home + "\nst=1\n"},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			p := presets[c.dialect]
			base := dialecttest.Base{Dir: t.TempDir(), Vars: map[string]string{"HOME": home}}
			out, st, err := p.Combined(t, base, `cd; echo "st=$?"`)
			if err != nil {
				t.Fatal(err)
			}
			if out != c.want || st != 0 {
				t.Errorf("said %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// The same subject through the other failure, which is what tells a fix from
// a special case for one errno: a home that is there and is a file still has
// no operand, and the panel still names HOME's value with the reason the
// system gave. Two of the four distinguish this reason from the one above,
// so a fix that hard-coded "no such file or directory" fails here.
func TestEachDialectNamesHomeWhenHomeIsNotADirectory(t *testing.T) {
	for _, c := range []struct {
		dialect string
		want    string
	}{
		{"bash", "bash: line 1: cd: %s: Not a directory\nst=1\n"},
		{"dash", "dash: 1: cd: can't cd to %s\nst=2\n"},
		{"ksh", "ksh: cd: %s: [Not a directory]\nst=1\n"},
		{"zsh", "zsh:cd:1: not a directory: %s\nst=1\n"},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			// A scratch tree the test invented, never the person's own home:
			// this row is about a HOME that cannot be moved to, and pointing
			// it anywhere real would make the case depend on the machine.
			dir := t.TempDir()
			home := filepath.Join(dir, "home-is-a-file")
			if err := os.WriteFile(home, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			p := presets[c.dialect]
			base := dialecttest.Base{Dir: dir, Vars: map[string]string{"HOME": home}}
			out, st, err := p.Combined(t, base, `cd; echo "st=$?"`)
			if err != nil {
				t.Fatal(err)
			}
			if want := fmt.Sprintf(c.want, home); out != want || st != 0 {
				t.Errorf("said %q status %d, want %q at 0", out, st, want)
			}
		})
	}
}

// `cd -` is the third form with no operand to name, and it names OLDPWD's
// value for the two dialects that report the failure at all rather than the
// word `-` this once printed. bash and zsh are left out: they answer this
// shape differently again — bash calls an OLDPWD it cannot use "not set" and
// zsh moves nowhere in silence — which is a branch and not a wording, so it
// is #1490 rather than a column here.
func TestCdDashNamesOldpwdRatherThanTheDash(t *testing.T) {
	const gone = "/no-such-oldpwd-1483"
	for _, c := range []struct {
		dialect string
		want    string
	}{
		{"dash", "dash: 1: cd: can't cd to " + gone + "\nst=2\n"},
		{"ksh", "ksh: cd: " + gone + ": [No such file or directory]\nst=1\n"},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			p := presets[c.dialect]
			base := dialecttest.Base{Dir: t.TempDir(), Vars: map[string]string{"OLDPWD": gone}}
			out, st, err := p.Combined(t, base, `cd -; echo "st=$?"`)
			if err != nil {
				t.Fatal(err)
			}
			if out != c.want || st != 0 {
				t.Errorf("said %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}
