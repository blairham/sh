// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// reservedRun runs src with PATH pointing at dir alone, so the only externals
// that exist are the ones the test made.
func reservedRun(t *testing.T, dir, src string, setup func(*Runner)) (string, int, error) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	// `command -v` on a name that is not there asks this, and these tests
	// reach it deliberately.
	sem.CommandNotFoundStatusIsNotFound = No
	dg := Diagnostics{}
	r := &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Dir: dir, Name: "testsh", Env: []string{"PATH=" + dir},
	}
	if setup != nil {
		setup(r)
	}
	st, rerr := r.Run(context.Background(), f)
	return buf.String(), st, rerr
}

// shadow writes an executable named like a shell builtin, the way macOS ships
// /usr/bin/umask and nine others.
func shadow(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// The bug: a builtin this shell does not have was looked for on PATH, found,
// and run in a child — which changed its own state and exited. The call
// succeeded and nothing happened.
func TestAMissingBuiltinIsNotLookedForOnPath(t *testing.T) {
	// `umask` is not here any more: it is a builtin now, so it is answered
	// rather than refused, and the dispatch reaches it before this check.
	// Its own protection is stronger and is tested below.
	// `ulimit` has left too, for the reason umask did: it is a builtin now.
	// `jobs`, `fg` and `bg` have left too: they are builtins now.
	// And `type`, for the same reason — its own protection is that it never
	// reports a reserved name's PATH hit, which TestTypeWillNotName covers.
	// `alias` and `unalias` have left as well: they keep a table now, and a
	// table kept in a child that then exits is the whole bug this guards.
	for _, name := range []string{"hash"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			// An external of the same name that would happily succeed.
			shadow(t, dir, name, "echo ran-the-external")
			out, _, err := reservedRun(t, dir, name+" 077", nil)
			if err == nil {
				t.Fatalf("%s: ran without complaint, output %q", name, out)
			}
			if strings.Contains(out, "ran-the-external") {
				t.Errorf("%s: ran the external on PATH: %q", name, out)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("%s: refusal does not name it: %v", name, err)
			}
		})
	}
}

// `fc` is the one of the ten that is not reserved: dash answers
// `command -v fc` with /usr/bin/fc, so it really is an external there and
// reserving it would be inventing a rule the panel does not have.
func TestFcIsNotReserved(t *testing.T) {
	dir := t.TempDir()
	shadow(t, dir, "fc", "echo ran-the-external")
	out, _, err := reservedRun(t, dir, "fc", nil)
	if err != nil {
		t.Fatalf("fc was refused: %v", err)
	}
	if !strings.Contains(out, "ran-the-external") {
		t.Errorf("said %q, want the external to have run", out)
	}
}

// Reserving a name is about never running a *child* for it, not about keeping
// it unimplemented — so a dialect that registers one stops it being reserved.
func TestRegisteringOneStopsItBeingReserved(t *testing.T) {
	dir := t.TempDir()
	shadow(t, dir, "hash", "echo ran-the-external")
	setup := func(r *Runner) {
		r.Register("hash", func(r *Runner, _ context.Context, _ []string) int {
			_, _ = r.Stdout.Write([]byte("the builtin ran\n"))
			return 0
		})
	}
	out, st, err := reservedRun(t, dir, "hash 077", setup)
	if err != nil {
		t.Fatalf("refused a registered builtin: %v", err)
	}
	if st != 0 || !strings.Contains(out, "the builtin ran") {
		t.Errorf("said %q status %d, want the registered builtin", out, st)
	}
	if strings.Contains(out, "ran-the-external") {
		t.Errorf("said %q, want the builtin rather than the external", out)
	}
}

// A name that is not reserved still resolves from PATH, which is most of them.
func TestAnOrdinaryNameStillResolvesFromPath(t *testing.T) {
	dir := t.TempDir()
	shadow(t, dir, "notabuiltin", "echo ran-the-external")
	out, st, err := reservedRun(t, dir, "notabuiltin", nil)
	if err != nil {
		t.Fatalf("refused an ordinary command: %v", err)
	}
	if st != 0 || !strings.Contains(out, "ran-the-external") {
		t.Errorf("said %q status %d, want the external", out, st)
	}
}

// A function of the same name still wins, because a function shadows both a
// builtin and an external and is checked before either.
func TestAFunctionStillShadowsAReservedName(t *testing.T) {
	dir := t.TempDir()
	shadow(t, dir, "hash", "echo ran-the-external")
	out, st, err := reservedRun(t, dir, "hash() { echo the-function; }\nhash 077", nil)
	if err != nil {
		t.Fatalf("refused a function: %v", err)
	}
	if st != 0 || !strings.Contains(out, "the-function") {
		t.Errorf("said %q status %d, want the function", out, st)
	}
}

// `command -v` has to answer for what will actually run.
//
// There is an executable called /usr/bin/hash and this shell refuses to run
// it, so reporting it would defeat the guard a careful script writes — and
// that guard exists to avoid exactly the failure that follows it.
//
// Written with `hash` rather than `umask` or `ulimit`: both are builtins now, so it is
// reported as one, which is the case TestCommandVReportsARegisteredReservedName
// covers.
func TestCommandVDoesNotAdvertiseAReservedExternal(t *testing.T) {
	dir := t.TempDir()
	shadow(t, dir, "hash", "echo ran-the-external")
	shadow(t, dir, "ordinary", "echo ran-the-external")

	out, _, err := reservedRun(t, dir, `command -v hash; echo "st=$?"`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "hash") || !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want silence and a failure", out)
	}
	// The guard, end to end: it must take the other branch and carry on
	// rather than passing and then dying.
	out, _, err = reservedRun(t, dir,
		"if command -v hash >/dev/null; then hash 077; else echo no-hash; fi\necho reached", nil)
	if err != nil {
		t.Fatalf("the guard did not protect the script: %v", err)
	}
	if !strings.Contains(out, "no-hash") || !strings.Contains(out, "reached") {
		t.Errorf("said %q, want the else branch and the script to carry on", out)
	}
	// An ordinary name is still reported by the path that would run.
	out, _, err = reservedRun(t, dir, `command -v ordinary`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, filepath.Join(dir, "ordinary")) {
		t.Errorf("said %q, want the external's path", out)
	}
}

// And a dialect that registers one gets it reported as a builtin, which is the
// clause that makes reserving conditional on not having it.
func TestCommandVReportsARegisteredReservedName(t *testing.T) {
	dir := t.TempDir()
	shadow(t, dir, "hash", "echo ran-the-external")
	setup := func(r *Runner) {
		r.Register("hash", func(*Runner, context.Context, []string) int { return 0 })
	}
	out, _, err := reservedRun(t, dir, `command -v hash`, setup)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "hash" {
		t.Errorf("said %q, want the builtin named", out)
	}
}

// `umask` is a builtin now, so it never reaches PATH — with or without a hook
// to change the process's mask. A shell that had no mask to offer and fell
// through to /usr/bin/umask is the whole of what #117 was about, and having
// the builtin is what makes that impossible rather than merely refused.
func TestUmaskNeverReachesPath(t *testing.T) {
	dir := t.TempDir()
	shadow(t, dir, "umask", "echo ran-the-external")
	out, st, err := reservedRun(t, dir, "umask 077", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "ran-the-external") {
		t.Errorf("said %q, want the external never to run", out)
	}
	// No hook, so it says so rather than pretending.
	if st == 0 {
		t.Errorf("status %d, want a failure when there is no umask to set", st)
	}
	// And with one, it is used.
	var got int
	setup := func(r *Runner) {
		r.SetUmask = func(mask int) (int, error) { old := got; got = mask; return old, nil }
	}
	if out, st, err := reservedRun(t, dir, "umask 077", setup); err != nil || st != 0 {
		t.Fatalf("with a hook: %q status %d err %v", out, st, err)
	}
	if got != 0o077 {
		t.Errorf("the hook was given %#o, want 077", got)
	}
}
