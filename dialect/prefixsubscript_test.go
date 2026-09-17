// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A subscripted assignment written as a command prefix, across the dialects,
// which is where it belongs: the panel answers it four ways and one table is
// the only place the split can be stated (#3433).
//
// Each row resets `arr=(x y z)` and writes `arr[2]=P` in front of a different
// kind of command, then prints the whole array. Measured 2026-09-16 under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, from a script file with stdin closed,
// and these dialect binaries match the references byte for byte on the same
// file:
//
//	command kind          bash 5.3.20            ksh93u+    zsh 5.9.2
//	a function            refused, `x y z`       `x y P`    `x P z`
//	a regular builtin     refused, `x y z`       `x y z`    `x P z`
//	`:`                   refused, `x y z`       `x y P`    `x P z`
//	an external           refused, `x y z`       `x y z`    `x y z`
//	`command` + external  refused, `x y z`       `x y z`    `x y z`
//
// zsh counts from one, so its `P` lands a column earlier — that is its
// ArrayBaseIsZero and not this question. What this question is: bash refuses
// the word and runs the command; ksh93 writes the element and gives it back
// exactly where it gives a scalar back; zsh writes it and keeps it past
// everything it runs itself. dash has no array and no subscripted assignment
// word, so its row is the fourth reading on a line of its own below.

func subscriptPrefixDialect(t *testing.T, sh driver.Shell, src string) (string, string, int) {
	t.Helper()
	var out, errs strings.Builder
	sh.Stdout, sh.Stderr = &out, &errs
	code := driver.MainArgs(sh, []string{sh.Name, "-c", src})
	return out.String(), errs.String(), code
}

func TestASubscriptedPrefixAcrossTheDialects(t *testing.T) {
	type want struct{ out, errs string }
	refused := "bash: line 1: `arr[2]': not a valid identifier\n"
	for _, row := range []struct {
		kind, cmd     string
		bash, ksh, zs want
	}{
		{
			"a function", "f",
			want{"[x y z]\n", refused},
			want{"[x y P]\n", ""},
			want{"[x P z]\n", ""},
		},
		{
			"a regular builtin", "read -r j </dev/null",
			want{"[x y z]\n", refused},
			want{"[x y z]\n", ""},
			want{"[x P z]\n", ""},
		},
		{
			"a special builtin", ":",
			want{"[x y z]\n", refused},
			want{"[x y P]\n", ""},
			want{"[x P z]\n", ""},
		},
		{
			"an external", "/usr/bin/true",
			want{"[x y z]\n", refused},
			want{"[x y z]\n", ""},
			want{"[x y z]\n", ""},
		},
		{
			"command before an external", "command /usr/bin/true",
			want{"[x y z]\n", refused},
			want{"[x y z]\n", ""},
			want{"[x y z]\n", ""},
		},
	} {
		src := `f() { :; }; arr=(x y z); arr[2]=P ` + row.cmd + `; echo "[${arr[*]}]"`
		for _, d := range []struct {
			sh   driver.Shell
			want want
		}{
			{bashShell(), row.bash},
			{kshShell(), row.ksh},
			{zshShell(), row.zs},
		} {
			t.Run(row.kind+"/"+d.sh.Name, func(t *testing.T) {
				out, errs, code := subscriptPrefixDialect(t, d.sh, src)
				if out != d.want.out || errs != d.want.errs || code != 0 {
					t.Errorf("%s\n got %q stderr %q status %d\nwant %q stderr %q status 0",
						src, out, errs, code, d.want.out, d.want.errs)
				}
			})
		}
	}
}

// The refusal costs the word and nothing else: the other entries of the same
// prefix reach the command, and the status is the command's own. Measured on
// bash 5.3.20 with a function, which is the one command kind that shows a
// prefix to the body — a regular builtin's prefix is given back whatever
// happened to it, so it cannot tell a refused word from a refused prefix.
func TestBashRefusesTheSubscriptedWordAndNotThePrefix(t *testing.T) {
	out, errs, code := subscriptPrefixDialect(t, bashShell(),
		`f() { echo "w=[$w]"; return 4; }; w=5 arr[1]=1 f; echo "st=$?"`)
	if want := "w=[5]\nst=4\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if want := "bash: line 1: `arr[1]': not a valid identifier\n"; errs != want || code != 0 {
		t.Errorf("stderr = %q status %d, want %q at 0", errs, code, want)
	}
}

// And dash's reading is the grammar's: the word is a command, not found.
// Nothing in the vector is asked, which is why the dialect leaves both axes
// unanswered — so a row here is what says the grammar still answers it.
func TestDashReadsASubscriptedWordAsACommand(t *testing.T) {
	out, errs, code := subscriptPrefixDialect(t, dashShell(), `f() { :; }; a[1]=v f; echo "st=$?"`)
	if want := "st=127\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if !strings.Contains(errs, "a[1]=v: not found") || code != 0 {
		t.Errorf("stderr = %q status %d, want the word reported not found", errs, code)
	}
}
