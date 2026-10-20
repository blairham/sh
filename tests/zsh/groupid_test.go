// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// This shell starts with four numeric identity parameters, not two.
//
// Measured 2026-09-25 against zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f -c`,
// on a machine whose uid and gid differ — `uid=501(bhamilton) gid=20(staff)`,
// which is what makes the row evidence rather than a coincidence a single
// number would produce either way:
//
//	print "UID=[$UID] EUID=[$EUID] GID=[$GID] EGID=[$EGID]"
//
//	zsh 5.9.2    UID=[501] EUID=[501] GID=[20] EGID=[20]
//	bash 5.3.20  UID=[501] EUID=[501] GID=[]   EGID=[]
//	bash 3.2.57  UID=[501] EUID=[501] GID=[]   EGID=[]
//
// **Unset is not a refusal.** A parameter that expands to nothing is dropped
// from the command line rather than complained about, so the preparation code
// of zsh's own `C02cond.ztst` — which hands `$EGID` to `chgrp` — ran the
// command one argument short and the *system's* `chgrp` answered with a
// `usage:` line. The whole file was then abandoned before its first condition
// test. That is why this is a bug and not a cosmetic gap, and why the visible
// symptom was a third-party program's message rather than any diagnostic of
// ours (#4476).
//
// The uid half is asserted here beside the gid half deliberately: this is one
// group of four parameters set through one seam, and a test that covered only
// the new pair would not notice the old pair leaving.
func TestTheFourIdentityParametersAreSetAtStartup(t *testing.T) {
	for _, tc := range []struct {
		name string
		want int
	}{
		{"UID", os.Getuid()},
		{"EUID", os.Geteuid()},
		{"GID", os.Getgid()},
		{"EGID", os.Getegid()},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
			`print -r -- "[${`+tc.name+`-no such parameter}]"`)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if want := "[" + strconv.Itoa(tc.want) + "]\n"; out != want {
			t.Errorf("$%s = %q, want %q — an unset one is silently dropped from a command line, not refused", tc.name, out, want)
		}
	}
}

// And a word built from one reaches the command as a real argument.
//
// This is the assertion the issue's bar is stated in, and it is deliberately
// *not* a search for a phrase: the `no such file or directory` and `usage:`
// lines that gave the bug away came out of the system's own `chmod` and
// `chgrp`, so grepping our diagnostics for them would have found nothing
// either before the fix or after it. What moved is the argument count the
// command was handed, so that is what is counted.
//
// A builtin stands in for the external program, because the fact under test
// is the expansion and not the utility: `print -r -- $#` inside a function
// given `$EGID` is the same question `chgrp $EGID file` asks, with no
// dependency on a group this machine happens to have.
func TestAGroupIDReachesACommandAsAWord(t *testing.T) {
	for _, name := range []string{"GID", "EGID"} {
		out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
			`count() { print -r -- "argc=$# argv1=$1"; }; count $`+name+` file`)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.HasPrefix(out, "argc=2 ") {
			t.Errorf("$%s as an argument gave %q, want two arguments — an empty word is dropped and the command runs one short", name, out)
		}
	}
}

// The environment may not supply any of the four.
//
// Measured on the same run: `env GID=999 zsh -f -c 'print $GID'` writes 20 and
// `env EGID=999 …` writes 20, exactly as `env UID=999 …` writes the real uid.
// This is the half a plain store does not give — the environment is a layer
// under the stored table, so a name found unstored may still be a name the
// shell was handed — and it is the opposite answer from `$HOST` beside it,
// which an inherited value does win (see host_test.go).
//
// It also parts this shell from bash, where the names are nothing special:
// `env GID=999 bash --norc -c 'echo $GID'` writes 999.
func TestTheEnvironmentCannotNameTheIdentityParameters(t *testing.T) {
	base := dialecttest.Base{
		Dir: t.TempDir(),
		Env: []string{"UID=999", "EUID=999", "GID=999", "EGID=999"},
	}
	out, _, err := preset.Combined(t, base, `print -r -- "[$UID][$EUID][$GID][$EGID]"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "[" + strconv.Itoa(os.Getuid()) + "][" + strconv.Itoa(os.Geteuid()) +
		"][" + strconv.Itoa(os.Getgid()) + "][" + strconv.Itoa(os.Getegid()) + "]\n"
	if out != want {
		t.Errorf("got %q, want %q — these are facts about the process, not names it inherits", out, want)
	}
}

// `unset` removes them, and the removal is not a refusal.
//
// Measured: `unset GID; print "rc=$? val=[$GID] t=${(t)GID}"` writes
// `rc=0 val=[] t=` — so these carry none of the read-only mark `$LINENO` does
// in this shell, and a script that removes one gets an ordinary unset name
// back.
func TestUnsettingAGroupIDRemovesIt(t *testing.T) {
	for _, name := range []string{"GID", "EGID"} {
		out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
			`unset `+name+`; print -r -- "rc=$? [${`+name+`-no such parameter}]"`)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if want := "rc=0 [no such parameter]\n"; out != want || st != 0 {
			t.Errorf("unset %s gave %q (status %d), want %q at 0", name, out, st, want)
		}
	}
}
