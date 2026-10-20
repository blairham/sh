// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"regexp"
	"strings"
	"testing"
)

// The parameters this shell supplies that were absent here, and the
// attributes it puts on its own.
//
// Measured 2026-09-18 on bash 5.3.20, `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with a scratch HOME, over a script file (#3098, #3099).

func TestTheShellsOwnParametersExist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct {
		src, want string
	}{
		{`echo "${BASHPID-NONE}"`, `^[0-9]+$`},
		{`echo "${SRANDOM-NONE}"`, `^[0-9]+$`},
		{`echo "${HISTCMD-NONE}"`, `^0$`},
		{`echo "${GROUPS[0]-NONE}"`, `^[0-9]+$`},
		{`echo "${DIRSTACK[0]-NONE}"`, `^/`},
	} {
		out, st := runBash(t, dir, row.src)
		if !regexp.MustCompile(row.want).MatchString(strings.TrimSpace(out)) || st != 0 {
			t.Errorf("%s = %q (status %d), want a match for %q", row.src, out, st, row.want)
		}
	}
	// Two reads of the entropy source are two different numbers, which is
	// what says it is not `$RANDOM` under another name.
	out, _ := runBash(t, dir, `a=$SRANDOM; b=$SRANDOM; [ "$a" != "$b" ] && echo differ`)
	if strings.TrimSpace(out) != "differ" {
		t.Errorf("two reads of SRANDOM = %q, want two different numbers", out)
	}
	// And each takes an assignment at 0 and discards it.
	for _, name := range []string{"BASHPID", "HISTCMD"} {
		out, st := runBash(t, dir, name+`=5; echo "st=$? now=$`+name+`"`)
		if out == "st=0 now=5\n" || st != 0 {
			t.Errorf("%s=5 = %q (status %d), want the assignment taken and discarded", name, out, st)
		}
		if !strings.HasPrefix(out, "st=0 ") {
			t.Errorf("%s=5 = %q, want status 0", name, out)
		}
	}
}

// How each lists back, which is the half a script reads when it dumps the
// state it is about to restore.
func TestTheShellsOwnParametersListWithTheirLetters(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct{ src, want string }{
		{`declare -p BASHPID`, `^declare -i BASHPID="[0-9]+"\n$`},
		{`declare -p SRANDOM`, `^declare -i SRANDOM="[0-9]+"\n$`},
		{`declare -p HISTCMD`, `^declare -i HISTCMD="0"\n$`},
		{`declare -p GROUPS`, `^declare -a GROUPS=\(\[0\]="[0-9]+"`},
		{`declare -p EUID`, `^declare -ir EUID="[0-9]+"\n$`},
		{`declare -p UID`, `^declare -ir UID="[0-9]+"\n$`},
		{`declare -p PPID`, `^declare -ir PPID="[0-9]+"\n$`},
		{`declare -p OPTIND`, `^declare -i OPTIND="1"\n$`},
		// The produced arrays, which the listing and the expansion used to
		// disagree about: `${BASH_SOURCE[0]}` answered and `declare -p
		// BASH_SOURCE` said the name was not there.
		{`declare -p BASH_SOURCE`, `^declare -a BASH_SOURCE=\(`},
		{`declare -p BASH_LINENO`, `^declare -a BASH_LINENO=\(`},
		{`declare -p BASH_ARGC`, `^declare -a BASH_ARGC=\(`},
		{`declare -p BASH_ARGV`, `^declare -a BASH_ARGV=\(`},
		{`true|false; declare -p PIPESTATUS`, `^declare -a PIPESTATUS=\(\[0\]="0" \[1\]="1"\)\n$`},
		// FUNCNAME is absent outside a call rather than empty, and the
		// listing says so with no `=` at all — the row that says nil and
		// empty are two answers.
		{`declare -p FUNCNAME`, `^declare -a FUNCNAME\n$`},
		{`f(){ declare -p FUNCNAME; }; f`, `^declare -a FUNCNAME=\(\[0\]="f"`},
	} {
		out, st := runBash(t, dir, row.src)
		if !regexp.MustCompile(row.want).MatchString(out) || st != 0 {
			t.Errorf("%s = %q (status %d), want a match for %q", row.src, out, st, row.want)
		}
	}
}

// The version array is the prelude's and is frozen on the line after it is
// filled in, which is where the freeze has to be: a mark made from Go before
// the prelude ran would refuse the very assignment that fills it.
func TestTheVersionArrayIsFrozen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runBashPrelude(t, dir, `declare -p BASH_VERSINFO`)
	if !regexp.MustCompile(`^declare -ar BASH_VERSINFO=\(`).MatchString(out) || st != 0 {
		t.Errorf("declare -p BASH_VERSINFO = %q (status %d), want the frozen array", out, st)
	}
	if out, st := runBashPrelude(t, dir, `BASH_VERSINFO=(1); echo reached`); strings.Contains(out, "reached") || st == 0 {
		t.Errorf("BASH_VERSINFO=(1) = %q (status %d), want it refused", out, st)
	}
}

// The freeze is not cosmetic: this shell refuses an assignment to three of
// its own, where every one of them was taken here at status 0.
func TestTheShellsOwnReadonlyParametersRefuseAnAssignment(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"EUID", "UID", "PPID"} {
		out, st := runBash(t, dir, name+`=0; echo reached`)
		if strings.Contains(out, "reached") || st == 0 {
			t.Errorf("%s=0 = %q (status %d), want it refused", name, out, st)
		}
	}
	// And the one that is not frozen, which is measured rather than
	// inferred: `getopts` writes it and a script resets it between scans.
	if out, st := runBash(t, dir, `OPTIND=3; declare -p OPTIND`); out != "declare -i OPTIND=\"3\"\n" || st != 0 {
		t.Errorf("OPTIND=3 = %q (status %d), want the write taken", out, st)
	}
}

// The directory stack is a **view** and not a store: slot zero is `$PWD` now
// rather than at push time, a write to slot N replaces entry N-1 where there
// is one, and the stack never grows or shrinks through the parameter.
func TestTheDirectoryStackParameterIsAView(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const push = `cd /tmp; pushd /usr >/dev/null; pushd /etc >/dev/null; `
	for _, row := range []struct{ src, want string }{
		{`cd /tmp; pushd /usr >/dev/null; cd /etc; echo "${DIRSTACK[0]}"`, "/etc\n"},
		{push + `DIRSTACK[1]=/var; echo "${DIRSTACK[*]}"`, "/etc /var /tmp\n"},
		{push + `DIRSTACK[0]=/zzz; echo "$PWD ${DIRSTACK[*]}"`, "/etc /etc /usr /tmp\n"},
		{push + `DIRSTACK=5; echo "${DIRSTACK[*]}"`, "/etc /usr /tmp\n"},
		{push + `DIRSTACK=(/a /b); echo "${DIRSTACK[*]}"`, "/etc /b /tmp\n"},
		{push + `DIRSTACK+=(/c); echo "${DIRSTACK[*]}"`, "/etc /usr /tmp\n"},
		{push + `unset 'DIRSTACK[2]'; echo "u=$? ${DIRSTACK[*]}"`, "u=0 /etc /usr /tmp\n"},
	} {
		out, st := runBashPrelude(t, dir, row.src)
		if out != row.want || st != 0 {
			t.Errorf("%s\n got %q (status %d)\nwant %q", row.src, out, st, row.want)
		}
	}
}

// The operand-less listing withholds a produced *reading* and writes the kind
// — `declare -i BASHPID` with no number — and a produced array's **elements
// are not a reading**: they are written, empty, beside the kind letter. One
// array is the exception and writes them in full.
//
// Measured 2026-09-18, a single bare `declare -p` after `true|false` in a
// shell that has read nothing (#3099).
func TestTheBareListingWithholdsAReadingAndNotAKind(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, st := runBash(t, dir, "true|false; declare -p")
	if st != 0 {
		t.Fatalf("declare -p answered %d: %q", st, out)
	}
	for _, want := range []string{
		// The scalars: the letter and no reading.
		`(?m)^declare -i BASHPID$`,
		`(?m)^declare -i SRANDOM$`,
		`(?m)^declare -i HISTCMD$`,
		// The arrays: the kind letter and an empty element list, from a
		// shell whose named `declare -p GROUPS` writes the group ids.
		`(?m)^declare -a GROUPS=\(\)$`,
		`(?m)^declare -a DIRSTACK=\(\)$`,
		`(?m)^declare -a BASH_SOURCE=\(\)$`,
		// The one that writes its elements here, which is a fact about the
		// name rather than a rule about views.
		`(?m)^declare -a PIPESTATUS=\(\[0\]="0" \[1\]="1"\)$`,
		// And absent is still absent: no `=` at all.
		`(?m)^declare -a FUNCNAME$`,
	} {
		if !regexp.MustCompile(want).MatchString(out) {
			t.Errorf("declare -p = %q, want a row matching %q", out, want)
		}
	}
}
