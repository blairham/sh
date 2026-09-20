// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// producedRows keeps the rows of a listing that name a parameter the shell
// produces, so a case can compare them as bytes without the machine's own
// environment in the way.
func producedRows(out string, names ...string) string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		for _, name := range names {
			if strings.Contains(line, " "+name+"=") || strings.HasSuffix(line, " "+name) {
				keep = append(keep, line)
				break
			}
		}
	}
	return strings.Join(keep, "\n")
}

// A declaration letter with no names walks the produced parameters too, and
// the letter is what decides which of them stay.
//
// Measured 2026-09-20 on bash 5.3.20, one letter per script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`, in a shell that has read nothing:
//
//	declare -a   BASH_ARGC BASH_ARGV BASH_LINENO BASH_SOURCE DIRSTACK FUNCNAME GROUPS
//	declare -A   BASH_ALIASES BASH_CMDS
//	declare -i   BASHPID RANDOM SRANDOM
//	declare -x   none
//	declare -r   none
//
// This shell wrote **none** of them from any of those letters, while its own
// unfiltered `declare -p` wrote every one — so the names were registered and
// the filtered walk was the one place that could not see them. `declare -A`
// wrote nothing at all, because the two association tables had no listing
// registered and so were in neither form.
//
// The `-x` and `-r` rows are the control, and they are not idle: no produced
// parameter carries either attribute in any column of the panel, so a change
// that admitted the produced names to every filtered walk rather than to the
// ones a *letter* asked for would put them into `export -p` and `readonly -p`
// as well.
func TestAnAttributeLetterListsTheProducedParametersItSelects(t *testing.T) {
	const arrays = "BASH_ARGC BASH_ARGV BASH_LINENO BASH_SOURCE DIRSTACK FUNCNAME GROUPS"
	for _, c := range []struct{ name, line, want string }{
		{"arrays", "declare -a", arrays},
		{"associations", "declare -A", "BASH_ALIASES BASH_CMDS"},
		{"integers", "declare -i", "BASHPID RANDOM SRANDOM"},
		{"the same letter beside -p", "declare -pa", arrays},
		// The two builtins whose listing is a filter of its own. Neither
		// attribute is on a produced parameter anywhere in the panel.
		{"exported names", "declare -x", ""},
		{"read-only names", "declare -r", ""},
		{"export -p", "export -p", ""},
		{"readonly -p", "readonly -p", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), c.line)
			if st != 0 {
				t.Fatalf("%q: status %d, want 0", c.line, st)
			}
			var got []string
			for _, name := range strings.Fields(arrays + " BASH_ALIASES BASH_CMDS BASHPID RANDOM SRANDOM") {
				if producedRows(out, name) != "" {
					got = append(got, name)
				}
			}
			if strings.Join(got, " ") != c.want {
				t.Errorf("%q named %q, want %q", c.line, strings.Join(got, " "), c.want)
			}
		})
	}
}

// Which produced arrays write their **elements** in an operand-less listing
// is a fact about each name, and two of the call-stack five are on the side
// that does.
//
// Measured 2026-09-20 on bash 5.3.20, a script file whose only line is
// `declare -p`, under `env -i PATH=/usr/bin:/bin LC_ALL=C`:
//
//	declare -a BASH_ARGC=()
//	declare -a BASH_ARGV=()
//	declare -a BASH_LINENO=([0]="0")
//	declare -a BASH_SOURCE=([0]="p.sh")
//	declare -a DIRSTACK=()
//	declare -a FUNCNAME
//	declare -a GROUPS=()
//
// The withholding is real rather than a set of empty parameters, which is
// what the control row says: the *named* `declare -p DIRSTACK` in the same
// run writes the directory the bare form left out. So the line is drawn per
// name, exactly as it already was for PIPESTATUS, and these two were on the
// wrong side of it.
//
// `declare -p BASH_ARGC` is deliberately not the control, and the reason is a
// second difference found on the way: bash 5.3.20 writes `([0]="0")` for it
// at a script's top level where this shell writes `()`, `shopt -s extdebug`
// off in both. That is #3887 and is not this.
func TestTheOperandLessListingWritesTheElementsOfTwoCallStackArrays(t *testing.T) {
	out := runWithScriptFile(t, "declare -a", "p.sh")
	for _, c := range []struct{ name, want string }{
		{"BASH_SOURCE", `declare -a BASH_SOURCE=([0]="p.sh")`},
		{"BASH_LINENO", `declare -a BASH_LINENO=([0]="0")`},
		{"BASH_ARGC", `declare -a BASH_ARGC=()`},
		{"BASH_ARGV", `declare -a BASH_ARGV=()`},
		{"DIRSTACK", `declare -a DIRSTACK=()`},
		{"FUNCNAME", `declare -a FUNCNAME`},
	} {
		if got := producedRows(out, c.name); got != c.want {
			t.Errorf("%s listed as %q, want %q", c.name, got, c.want)
		}
	}
	// The control, and the reason the rows above are a withholding rather
	// than a set of empty parameters: asked for by *name*, the same run of
	// the same shell writes the elements. DIRSTACK carries the current
	// directory, so it is the one of the four whose contents this test can
	// state without knowing the machine.
	named := runWithScriptFile(t, `declare -p DIRSTACK`, "p.sh")
	if bare := producedRows(out, "DIRSTACK"); named == bare || !strings.Contains(named, `[0]="`) {
		t.Errorf("named DIRSTACK listed as %q against the bare %q; want its elements", named, bare)
	}
}
