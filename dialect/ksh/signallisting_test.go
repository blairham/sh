// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

func runKshSignals(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// TestTheBareListingWritesTheOlderNameAndTheUnnamedPosition is #3536.
//
// Measured 2026-09-17 on macOS arm64 against ksh93u+ (AJM 93u+ 2012-08-01),
// `kill -l` from a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`,
// newlines shown as `|`:
//
//	ksh93   HUP|INT|QUIT|ILL|TRAP|IOT|EMT|…|WINCH|SIG29|USR1|USR2|
//
// Two positions of that listing were wrong here and each is its own question.
//
// The sixth entry is **IOT**, an older name for the signal every other column
// calls ABRT. It is not a second number: `kill -l 6` is `ABRT` in this shell
// too, and `kill -l IOT` is `6`, so the word is read everywhere a name is
// read and written only where the shell is the one naming the signal — which
// is both listings, since `trap 'x' ABRT; trap` writes IOT as well.
//
// The position for signal 29 is **SIG29**, and this shell dropped it. Its own
// table has no name for 29 where every other column on this machine says INFO
// (Semantics.SignalNamesTheShellLacks), and the listing walks the platform's
// table rather than the dialect's for exactly that reason. The translating
// form spells the same gap differently — `kill -l 29` is the bare `29` — so
// the two need two fields.
//
// dash is the other column with a short table and writes the bare number in
// its listing, which is what makes the rendering a field rather than a rule.
func TestTheBareListingWritesTheOlderNameAndTheUnnamedPosition(t *testing.T) {
	out, st := runKshSignals(t, `kill -l`)
	if st != 0 {
		t.Fatalf("kill -l reported %d, want 0", st)
	}
	names := strings.Fields(out)
	if len(names) < 31 {
		t.Fatalf("kill -l wrote %d names, want the platform's table: %q", len(names), out)
	}
	if names[5] != "IOT" {
		t.Errorf("signal 6 is listed as %q, want IOT", names[5])
	}
	if !strings.Contains(out, "SIG29") {
		t.Errorf("kill -l wrote %q, want SIG29 in it for the position this table cannot name", out)
	}
	if strings.Contains(out, "ABRT") {
		t.Errorf("kill -l wrote %q, want no ABRT in it — this shell lists 6 as IOT", out)
	}
}

// TestTheOlderNameIsReadWhereverANameIsRead is the other half of #3536: the
// alias is the table's and not the listing's.
//
// Measured 2026-09-17 on the same binary. `kill -l IOT` is `6`; `kill -s IOT
// 999999` and `kill -IOT 999999` each reach a real send and report `kill:
// 999999: no such process` at 1; `trap 'echo x' IOT` is 0. bash 5.3.20
// answers none of them — `IOT: invalid signal specification` — and zsh 5.9.2
// answers all four, which is why the reading is an axis and the writing is
// this dialect's alone.
//
// And the number still translates back to the table's own name: `kill -l 6`
// is `ABRT` here, in the same shell whose listing writes IOT.
func TestTheOlderNameIsReadWhereverANameIsRead(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`kill -l IOT`, "6"},
		{`kill -l 6`, "ABRT"},
		{`kill -l iot`, "6"},
		{`kill -l SIGIOT`, "6"},
	} {
		out, st := runKshSignals(t, c.src)
		if st != 0 || strings.TrimSpace(out) != c.want {
			t.Errorf("%s said %q at %d, want %q at 0", c.src, out, st, c.want)
		}
	}
	// A trap set under the older name is the ABRT slot, and the listing
	// writes the shell's own word for it whichever spelling set it.
	out, st := runKshSignals(t, `trap 'echo x' IOT; trap`)
	if st != 0 {
		t.Fatalf("trap 'echo x' IOT reported %d, want 0", st)
	}
	if want := "trap -- 'echo x' IOT\n"; out != want {
		t.Errorf("trap listed %q, want %q", out, want)
	}
	out, _ = runKshSignals(t, `trap 'echo x' ABRT; trap`)
	if want := "trap -- 'echo x' IOT\n"; out != want {
		t.Errorf("a trap set as ABRT listed %q, want %q", out, want)
	}
}

// TestTheNumericOptionsRefusalsAreTheirOwn is #3544's second and third rows.
//
// Measured 2026-09-18 against ksh93u+ 2012-08-01, script file, `env -i
// PATH=/usr/bin:/bin LC_ALL=C`:
//
//	kill -n 9x 999999     the usage block and nothing else   2
//	kill -n NOPE 999999   the same                           2
//	kill -s                -s: signame argument expected + usage        2
//	kill -n                -n: numeric signum argument expected + usage 2
//
// `-n` takes a *numeric* signum here, so a word that is not one is a misuse
// of the option rather than a signal nobody has — the usage error a bare
// `kill` draws, at that status and unprefixed like it. This shell reached the
// unknown-signal sentence instead, which is the wording for `-s NOPE` and a
// different route in the reference.
//
// And the two options want different nouns when nothing follows them, which
// one field could not say. bash, zsh and dash word the two alike.
func TestTheNumericOptionsRefusalsAreTheirOwn(t *testing.T) {
	usage := "Usage: kill [-lL] [-n signum] [-s signame] job ...\n" +
		"   Or: kill [ options ] -l [arg ...]\n"
	for _, src := range []string{`kill -n 9x 999999`, `kill -n NOPE 999999`} {
		out, st := runKshSignals(t, src)
		if out != usage {
			t.Errorf("%s said %q, want the usage block alone", src, out)
		}
		if st != 2 {
			t.Errorf("%s reported %d, want 2", src, st)
		}
	}
	for _, c := range []struct{ src, want string }{
		{`kill -s`, "kill: -s: signame argument expected\n"},
		{`kill -n`, "kill: -n: numeric signum argument expected\n"},
	} {
		out, st := runKshSignals(t, c.src)
		if !strings.Contains(out, c.want) {
			t.Errorf("%s said %q, want %q in it", c.src, out, c.want)
		}
		if !strings.HasSuffix(out, usage) {
			t.Errorf("%s said %q, want the usage block after it", c.src, out)
		}
		if st != 2 {
			t.Errorf("%s reported %d, want 2", c.src, st)
		}
	}
}
