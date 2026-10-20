// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// `newgrp` is a builtin here, and a **special** one, because it is `exec
// newgrp` under a name: a command that replaces the shell has no status to
// hand back and nothing after it to run. See newgrp.go for the measurement
// (#3316).

// The roster holds the name, asserted on its own because the sentence below
// comes from it and a roster that had quietly lost the name would show there
// as an ordinary builtin rather than as a missing one.
func TestTheSpecialRosterHoldsNewgrp(t *testing.T) {
	names := strings.Fields(ksh.Semantics().SpecialBuiltinsBeyondPosix)
	for _, want := range []string{"alias", "unalias", "typeset", "newgrp"} {
		if !slicesContains(names, want) {
			t.Errorf("SpecialBuiltinsBeyondPosix = %q, with no %q in it",
				ksh.Semantics().SpecialBuiltinsBeyondPosix, want)
		}
	}
}

func slicesContains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// The sentence `type` and `command -V` write, which is the half of #3316 that
// a script can see without running anything: the word answered as a PATH hit
// here and as a special shell builtin there.
func TestTypeCallsNewgrpASpecialBuiltin(t *testing.T) {
	dir := t.TempDir()
	out, st := runKsh(t, dir, "type newgrp\ncommand -V newgrp\nwhence -v newgrp\n")
	if st != 0 {
		t.Fatalf("status = %d, want 0 — output %q", st, out)
	}
	if n := strings.Count(out, "newgrp is a special shell builtin"); n != 3 {
		t.Errorf("all three spellings should say so, got %d in:\n%s", n, out)
	}
}

// And the builtin is `exec` under a name, which is what the roster's third
// consequence — a failure being fatal — is unreachable *because of*. The
// program is the one `PATH` names, so a fixture stands in for it: the shell
// runs it, and nothing after the call runs, exactly as a replacement leaves
// the script.
func TestNewgrpTakesTheExecRoad(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// A stand-in for the real program: it says what it was given and exits,
	// which is all the road needs to be visible. Never the machine's own
	// `newgrp`, which changes a process's group.
	prog := "#!/bin/sh\nprintf 'ran with [%s]\\n' \"$*\"\nexit 7\n"
	if err := os.WriteFile(filepath.Join(bin, "newgrp"), []byte(prog), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "PATH=" + bin + "\nprintf start\nnewgrp -l somegroup\nprintf NOT-REACHED\n"
	out, st := runKsh(t, dir, src)
	if !strings.Contains(out, "ran with [-l somegroup]") {
		t.Errorf("the operands should reach the program, got %q", out)
	}
	if strings.Contains(out, "NOT-REACHED") {
		t.Errorf("nothing after the call should run, got %q", out)
	}
	if st != 7 {
		t.Errorf("status = %d, want the program's own 7 — output %q", st, out)
	}
}

// The name is looked up as a *word* rather than as a path, so a shell whose
// `PATH` has no such program says what it says for any other missing command
// and does not reach for the machine's.
func TestNewgrpWithNothingToRunIsACommandNotFound(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	out, st := runKsh(t, dir, "PATH="+empty+"\nnewgrp\nprintf NOT-REACHED\n")
	if st == 0 || strings.Contains(out, "NOT-REACHED") {
		t.Errorf("got %q/%d, want a refusal that ends the script", out, st)
	}
	if !strings.Contains(out, "newgrp") {
		t.Errorf("the refusal should name the word, got %q", out)
	}
}

// And the dialect's own vector still answers the two axes the roster's other
// consequences hang on, so a reader can tell "the name is special" from "the
// consequences are switched off".
func TestTheRosterConsequencesAreStillAnswered(t *testing.T) {
	s := ksh.Semantics()
	if s.TypeDistinguishesSpecialBuiltins != interp.Yes {
		t.Errorf("TypeDistinguishesSpecialBuiltins = %v, want yes", s.TypeDistinguishesSpecialBuiltins)
	}
	if s.AssignmentPrefixPersistsOnSpecialBuiltin != interp.Yes {
		t.Errorf("AssignmentPrefixPersistsOnSpecialBuiltin = %v, want yes",
			s.AssignmentPrefixPersistsOnSpecialBuiltin)
	}
}
