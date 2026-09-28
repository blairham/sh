// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// probeLogin is the login name these tests inject.
//
// **Never the real one.** `interp.LoginName` asks the system who this process
// is, and a test that read it would pass because it agreed with whoever ran
// it — which is a test of the machine rather than of the shell, and passes
// identically for a shell that answered the wrong question. Every row below
// that names a value names this one.
const probeLogin = "probe-login-name"

// identityRunner is a shell whose login name is the test's, built the way
// Apply builds one so that every other parameter is in place around it.
func identityRunner(t *testing.T, out io.Writer, env ...string) *interp.Runner {
	t.Helper()
	sem, diag, dl := Semantics(), Diagnostics(), Dialect()
	r := &interp.Runner{
		Stdout: out, Stderr: out,
		Semantics: &sem, Diagnostics: &diag, Dialect: &dl,
		// **An environment of the test's own, always.** A Runner with a nil
		// Env reads the process's, so a row here would have been graded
		// against whoever ran the test — and `$LOGNAME` is exported in a
		// real session, which is exactly the name these rows are about. The
		// first entry is a placeholder so the slice is never empty and
		// never falls back.
		Dir: t.TempDir(), Name: "zsh",
		Env: append([]string{"SH_IDENTITY_TEST=1"}, env...),
	}
	Apply(r)
	// Registered a second time with the test's own question in place of the
	// system's — and the producers Apply just installed are **taken off
	// first**, which is not tidiness. `registerTheIdentityValues` reads the
	// value the environment handed in for `LOGNAME`, and it reads it through
	// `GetVar` — which the producer Apply installed a moment ago would
	// answer, from the system. So a second registration over a live first
	// one records the machine's login name as an inherited value and every
	// row below reads the real user. The subject of the measurement was
	// repairing the probe; taking the producers off is what stops it.
	for _, name := range [...]string{"LOGNAME", "USERNAME", "TTY", "ZSH_SCRIPT"} {
		r.UnsetDynamic(name)
	}
	registerTheIdentityValues(r, func() string { return probeLogin })
	return r
}

func runIdentity(t *testing.T, src string, env ...string) string {
	t.Helper()
	f, err := syntax.Parse(src, Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	r := identityRunner(t, &out, env...)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out.String()
}

// `$USERNAME` and `$LOGNAME` are two names and not one value under two
// spellings, which is the trap #4903 records: measured on this machine in one
// shell under `env -i`, the reference answers `root` for the first and
// `bhamilton` for the second, so a guess at either would have been wrong in a
// way no test here would catch.
//
// What this pins is the rule each follows. `$USERNAME` is the process's login
// name and ignores the environment; `$LOGNAME` takes the environment's value
// when it was handed one and falls back to the same login name when it was
// not.
func TestTheTwoLoginNamesFollowDifferentRules(t *testing.T) {
	out := runIdentity(t, `print -r -- "USERNAME=$USERNAME"; print -r -- "LOGNAME=$LOGNAME"`)
	want := "USERNAME=" + probeLogin + "\nLOGNAME=" + probeLogin + "\n"
	if out != want {
		t.Errorf("with no environment: %q, want %q", out, want)
	}

	out = runIdentity(t,
		`print -r -- "USERNAME=$USERNAME"; print -r -- "LOGNAME=$LOGNAME"`,
		"LOGNAME=handed-in")
	want = "USERNAME=" + probeLogin + "\nLOGNAME=handed-in\n"
	if out != want {
		t.Errorf("with LOGNAME in the environment: %q, want %q — the environment wins for LOGNAME alone", out, want)
	}

	// And the environment does not reach `$USERNAME`, which is the other half
	// of the same row and the one a rule read off `$LOGNAME` would get wrong.
	out = runIdentity(t, `print -r -- "USERNAME=$USERNAME"`, "USERNAME=handed-in")
	if out != "USERNAME="+probeLogin+"\n" {
		t.Errorf("with USERNAME in the environment: %q, want the process's own name", out)
	}

	// A script's own assignment is the later word for the one that takes one.
	out = runIdentity(t, `LOGNAME=written; print -r -- "LOGNAME=$LOGNAME"`, "LOGNAME=handed-in")
	if out != "LOGNAME=written\n" {
		t.Errorf("after an assignment: %q, want the script's word", out)
	}
}

// The system is asked **once**, however many of the three readers look.
//
// `interp.LoginName` measures 0.83-1.10 ms on darwin and `Apply` runs on
// every invocation, so this is the property #1403 is about: one lookup for
// `%n`, `$USERNAME` and `$LOGNAME` together, and none at all for a shell that
// reads neither.
func TestTheLoginNameIsAskedOnceForEveryReader(t *testing.T) {
	asked := 0
	sem, diag, dl := Semantics(), Diagnostics(), Dialect()
	var out bytes.Buffer
	r := &interp.Runner{
		Stdout: &out, Stderr: &out,
		Semantics: &sem, Diagnostics: &diag, Dialect: &dl,
		Dir: t.TempDir(), Name: "zsh",
		Env: []string{"SH_IDENTITY_TEST=1"},
	}
	Apply(r)
	for _, name := range [...]string{"LOGNAME", "USERNAME", "TTY", "ZSH_SCRIPT"} {
		r.UnsetDynamic(name)
	}
	once := sync.OnceValue(func() string { asked++; return probeLogin })
	r.SetPromptUserFunc(once)
	registerTheIdentityValues(r, once)

	// Nothing has read a name yet, which is the half that pays for the rest:
	// a parameter that asked at startup would have counted one here.
	if asked != 0 {
		t.Fatalf("asked %d times before any read, want 0", asked)
	}
	f, err := syntax.Parse(`print -rP -- "%n"; print -r -- "$USERNAME $LOGNAME $USERNAME"`, Dialect())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if asked != 1 {
		t.Errorf("asked %d times for four reads, want 1", asked)
	}
	if !strings.Contains(out.String(), probeLogin) {
		t.Fatalf("output %q holds no login name at all — the readers are not reading", out.String())
	}
}
