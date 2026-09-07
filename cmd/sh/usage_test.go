// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"
)

// helped invokes the binary the way a person does and returns both streams,
// through run() rather than through a copy of what main does — the same reason
// the sandbox tests go that way.
func helped(t *testing.T, args ...string) (out, errs string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	code = run(append([]string{"sh"}, args...), nil, &o, &e)
	return o.String(), e.String(), code
}

// TestHelpIsAnsweredWithoutConsultingAnAxis is the whole point of #816's third
// question. A usage message is not shell behavior, so it must not be reachable
// only through a shell that works: `sh -h` used to be refused because whether
// `-h` is an option letter at all is an axis the panel disagrees about, which
// made the one binary a person cannot casually run also the one that would not
// say how to run it.
func TestHelpIsAnsweredWithoutConsultingAnAxis(t *testing.T) {
	t.Parallel()
	for _, spelling := range []string{"-h", "-help", "--help"} {
		t.Run(spelling, func(t *testing.T) {
			t.Parallel()
			out, errs, code := helped(t, spelling)
			if code != 0 {
				t.Errorf("status %d, want 0: help was what was asked for", code)
			}
			if errs != "" {
				t.Errorf("stderr = %q, want help on standard output alone", errs)
			}
			if !strings.Contains(out, "-dialect") {
				t.Errorf("output = %q, want the dialect flag named", out)
			}
			if strings.Contains(out, "no dialect was chosen") {
				t.Errorf("output = %q: help must not consult an axis", out)
			}
		})
	}
}

// TestHelpOutranksEverythingItCouldBeAskedAlongside is the same rule taken to
// its edge. Whatever else is wrong with the line — a dialect that does not
// exist, a flag missing its value — the answer to "how do I invoke this" is
// still the usage message. A binary that reports the mistake instead is a
// binary that will not tell you how to stop making it.
func TestHelpOutranksEverythingItCouldBeAskedAlongside(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"an unknown dialect", []string{"-dialect", "nosuchshell", "-h"}},
		{"a dialect flag with nothing after it", []string{"-h", "-dialect"}},
		{"help before a command that would have been refused", []string{"-h", "-c", "test a == a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, errs, code := helped(t, tc.args...)
			if code != 0 || errs != "" {
				t.Errorf("status %d stderr %q, want the usage message and nothing else", code, errs)
			}
			if !strings.Contains(out, "Usage:") {
				t.Errorf("output = %q, want the usage message", out)
			}
		})
	}
}

// TestHelpIsOnlyTakenFromTheFrontOfTheLine keeps the flag inside the boundary
// every other flag of this binary's is inside: the scan stops at the first word
// that is not ours, so a script's own `-h` stays the script's.
func TestHelpIsOnlyTakenFromTheFrontOfTheLine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"a script's parameter", []string{"x.sh", "-h"}},
		{"an operand after -c", []string{"-c", "echo hi", "-h"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			own, rest, err := readOwnFlags(tc.args)
			if err != nil {
				t.Fatalf("readOwnFlags: %v", err)
			}
			if own.help {
				t.Errorf("%v: a -h past the shell's first word is the shell's", tc.args)
			}
			if len(rest) != len(tc.args) {
				t.Errorf("rest = %v, want the whole line left to the shell", rest)
			}
		})
	}
}

// TestTheUsageNamesEveryFlagThisBinaryReads is a drift guard, and it guards the
// direction that goes wrong: a flag renamed or dropped while the usage text
// keeps advertising it, which is a message that lies rather than one that is
// merely incomplete. Each name is asserted from both sides — the scanner takes
// it, and the message mentions it — so neither can move alone.
func TestTheUsageNamesEveryFlagThisBinaryReads(t *testing.T) {
	t.Parallel()
	// Value flags are given one, so that a scan which takes the next word does
	// not run off the end of the line.
	for _, tc := range []struct {
		flag string
		args []string
	}{
		{"-dialect", []string{"-dialect", "bash"}},
		{"-tokens", []string{"-tokens"}},
		{"-parse", []string{"-parse"}},
		{"-policy", []string{"-policy", "p"}},
		{"-deny", []string{"-deny", "/x"}},
		{"-audit", []string{"-audit", "log"}},
		{"-trace-events", []string{"-trace-events"}},
		{"-blocks-list", []string{"-blocks-list"}},
		{"-blocks-show", []string{"-blocks-show", "1"}},
		{"-acp", []string{"-acp"}},
		{"-acp-connect", []string{"-acp-connect"}},
		{"-acp-allow", []string{"-acp-allow"}},
		{"-acp-auth", []string{"-acp-auth", "m"}},
		{"-help", []string{"-help"}},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			t.Parallel()
			_, rest, err := readOwnFlags(tc.args)
			if err != nil {
				t.Fatalf("readOwnFlags(%v): %v", tc.args, err)
			}
			if len(rest) != 0 {
				t.Errorf("%s was not read as this binary's own: %v left", tc.flag, rest)
			}
			if !strings.Contains(usageText, tc.flag) {
				t.Errorf("the usage message does not mention %s", tc.flag)
			}
		})
	}
}

// TestTheRefusalNamesTheRemedy is #816's second question. The diagnostic said
// exactly what was wrong and nothing about what to do, and the flag that would
// fix it is this binary's — so this binary is where the sentence is written and
// this is where it has to be seen arriving.
func TestTheRefusalNamesTheRemedy(t *testing.T) {
	t.Parallel()
	// An axis the default dialect refuses, reached through the front end.
	_, errs, code := helped(t, "-c", "test a == a")
	if code == 0 {
		t.Fatalf("status %d, want the refusal (stderr %q)", code, errs)
	}
	if !strings.Contains(errs, "no dialect was chosen") {
		t.Fatalf("stderr = %q, want the axis refused", errs)
	}
	if !strings.Contains(errs, "-dialect") {
		t.Errorf("stderr = %q, want the flag that would answer it named", errs)
	}
	// Naming `core` back would be offering to change nothing: it is the
	// default, and it is what produced the refusal.
	remedy := errs[strings.Index(errs, "no dialect was chosen"):]
	if strings.Contains(remedy, "core") {
		t.Errorf("remedy = %q, want it to offer only dialects that answer", remedy)
	}
	// And the shells it offers are ones -dialect actually takes, so following
	// the advice works rather than producing a second refusal.
	for _, name := range []string{"posix", "bash", "zsh", "ksh", "dash"} {
		if !strings.Contains(remedy, name) {
			t.Errorf("remedy = %q, missing %s", remedy, name)
		}
		if _, err := pickDialect(name); err != nil {
			t.Errorf("the remedy offers %s, which -dialect refuses: %v", name, err)
		}
	}
}

// TestTheDefaultDialectStaysTheCore records the first of #816's three answers,
// which is a "no". The obvious candidate for a default is `posix`, and it is
// measurably worse: it refuses at the *grammar* what every shell in the panel
// but one accepts, so a person would meet a parse failure on ordinary lines
// instead of an honest refusal at a disputed one.
func TestTheDefaultDialectStaysTheCore(t *testing.T) {
	t.Parallel()
	own, _, err := readOwnFlags(nil)
	if err != nil {
		t.Fatal(err)
	}
	if own.dialect != "core" {
		t.Errorf("default dialect = %q, want core", own.dialect)
	}
	for _, src := range []string{`a=(1 2 3)`, `[[ -n x ]]`, `s=abcdef; echo ${s:1:3}`} {
		_, errs, code := helped(t, "-dialect", "posix", "-c", src)
		if code == 0 && errs == "" {
			t.Errorf("posix ran %q: if it accepts this, reconsider the default", src)
		}
	}
}
