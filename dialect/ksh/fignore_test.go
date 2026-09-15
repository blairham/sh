// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// ignoreFixture is the directory every row below expands in: two names a
// pattern can take out, one it cannot, two hidden ones, and a subdirectory
// with its own.
//
// A scratch directory and nothing else — no home, no real user, no PATH off
// this tree — for the reason internal/testenv exists.
func ignoreFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub", "inner"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a.txt", "b.txt", "c.log", ".dot", ".hid.txt", "sub/x.txt", "sub/.y"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(f)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func runInFixture(t *testing.T, dir string, env []string, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": dir}, Env: env,
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// `FIGNORE` is the facility bash spells `GLOBIGNORE`, and it is the same
// facility answered differently in three places.
//
// Measured on ksh93u+, 2026-09-14, in the fixture above. This dialect had no
// such parameter at all, so every row was a no-op (#2748).
func TestTheIgnoreParameterIsFIGNORE(t *testing.T) {
	dir := ignoreFixture(t)
	for _, tc := range []struct{ name, src, want string }{
		{
			"a pattern takes names out and the hidden ones come in",
			`FIGNORE='*.txt'; printf "[%s]" *`,
			"[.][..][.dot][c.log][sub]",
		},
		// The value is **one pattern**, colons and all. No name here holds a
		// colon, so this takes nothing out — where bash's colon-separated
		// reading would take both.
		{
			"the value is one pattern and not a list",
			`FIGNORE='*.txt:*.log'; printf "[%s]" *`,
			"[.][..][.dot][.hid.txt][a.txt][b.txt][c.log][sub]",
		},
		// And the alternation a script wants there is the pattern grammar's,
		// which is the row that says the one above is not simply broken.
		{
			"an alternation is written as a pattern",
			`FIGNORE='@(*.txt|*.log)'; printf "[%s]" *`,
			"[.][..][.dot][sub]",
		},
		// The subject is the **entry in the directory**: no `./` in front of
		// it and no `/` behind it, and a pattern with a `/` in it reaches
		// nothing.
		{"the subject is the entry, not the word", `FIGNORE='*.txt'; printf "[%s]" sub/*`, "[sub/.][sub/..][sub/.y][sub/inner]"},
		{"a pattern naming the path reaches nothing", `FIGNORE='sub/x.txt'; printf "[%s]" sub/*`, "[sub/.][sub/..][sub/.y][sub/inner][sub/x.txt]"},
		{"the `./` a pattern wrote is not matched", `FIGNORE='./a.txt'; printf "[%s]" ./*`, "[./.][./..][./.dot][./.hid.txt][./a.txt][./b.txt][./c.log][./sub]"},
		{"and the bare name is", `FIGNORE='a.txt'; printf "[%s]" ./*`, "[./.][./..][./.dot][./.hid.txt][./b.txt][./c.log][./sub]"},
		{"a trailing slash belongs to the word and not to the entry", `FIGNORE='sub/'; printf "[%s]" */`, "[../][./][sub/]"},
		{"so the bare name takes the directory out", `FIGNORE='sub'; printf "[%s]" */`, "[../][./]"},
		// `.` and `..` are entries like any other here, so a pattern takes
		// them out too.
		{"the two dot names are filtered like the rest", `FIGNORE='.*'; printf "[%s]" *`, "[a.txt][b.txt][c.log][sub]"},
		{"one at a time", `FIGNORE='.'; printf "[%s]" *`, "[..][.dot][.hid.txt][a.txt][b.txt][c.log][sub]"},
		// A null value still shows the hidden names, where bash's does not.
		{"a null value shows the hidden names", `FIGNORE=''; printf "[%s]" *`, "[.][..][.dot][.hid.txt][a.txt][b.txt][c.log][sub]"},
		// And a value that matches nothing shows them too, which is what
		// separates "the parameter is set" from "a pattern matched".
		{"and so does one that matches nothing", `FIGNORE=zzz; printf "[%s]" *`, "[.][..][.dot][.hid.txt][a.txt][b.txt][c.log][sub]"},
		// Unset, the facility is off again — the control that keeps
		// "follows the parameter" from meaning "never goes off".
		{"unset puts it back", `FIGNORE='*.txt'; unset FIGNORE; printf "[%s]" *`, "[a.txt][b.txt][c.log][sub]"},
		{"and nothing set is nothing filtered", `printf "[%s]" *`, "[a.txt][b.txt][c.log][sub]"},
		// It reaches pathname expansion alone.
		{"a case arm is not filtered", `FIGNORE='*.txt'; case a.txt in *.txt) echo arm;; *) echo no;; esac`, "arm\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runInFixture(t, dir, nil, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// A value inherited from the environment is read, where bash's is not.
//
// It is the half of the model that cannot be seen from inside the script, and
// it is the reason the facility is not a switch an assignment latches here:
// there is nothing to latch, because this shell has no hidden-name option of
// its own for a script to write back.
func TestAnInheritedIgnoreValueIsRead(t *testing.T) {
	dir := ignoreFixture(t)
	out, st := runInFixture(t, dir, []string{"FIGNORE=*.txt"}, `printf "[%s]" *`)
	if want := "[.][..][.dot][c.log][sub]"; out != want || st != 0 {
		t.Errorf("gave %q at %d, want %q at 0", out, st, want)
	}
	// And a name that is not the parameter is still nothing to do with it.
	out, st = runInFixture(t, dir, []string{"GLOBIGNORE=*.txt"}, `printf "[%s]" *`)
	if want := "[a.txt][b.txt][c.log][sub]"; out != want || st != 0 {
		t.Errorf("under the other shell's name gave %q at %d, want %q at 0", out, st, want)
	}
}

// `.` and `..` are in what a pattern may match here, whether or not the
// ignore parameter is set — the leading-period rule is what keeps them out of
// an ordinary `*`.
//
// It is filed with `FIGNORE` because that is where it shows (#2748) and it is
// its own axis: `echo .*` answers `. .. .dot` with nothing set at all, where
// bash 5.3 and zsh answer `.dot`. bash 3.2 and dash agree with this shell.
func TestADotAndDotDotAreInTheListing(t *testing.T) {
	dir := ignoreFixture(t)
	for _, tc := range []struct{ name, src, want string }{
		{"a leading-period pattern reaches them", `printf "[%s]" .*`, "[.][..][.dot][.hid.txt]"},
		{"and only the directories with a slash", `printf "[%s]" .*/`, "[../][./]"},
		{"an ordinary star does not", `printf "[%s]" *`, "[a.txt][b.txt][c.log][sub]"},
		{"a subdirectory has them too", `printf "[%s]" sub/.*`, "[sub/.][sub/..][sub/.y]"},
		// A pattern that names them explicitly reaches them, which is what
		// says they are entries rather than a shape the `.*` pattern has.
		{"named outright", `printf "[%s]" ..`, "[..]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runInFixture(t, dir, nil, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
