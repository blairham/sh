// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// The fresh shell a shebang-less file runs in is built from the caller's
// exported fields, and two of those carry state a *script* can move: an
// extended-pattern option lives in syntax.Dialect, and several shell options
// in Semantics. Copied across, they made such a script inherit option changes
// a real fresh shell has no way to know about.
//
// Measured 2026-09-23 on bash 5.3.15 with an executable file holding
// `shopt extglob` and no shebang, run from a shell that had just set it: bash
// answers `off` — the parent's set does not carry — and this shell answered
// `on`. The leak was exactly that narrow; variables, functions and `set -e`
// were all correctly fresh in the same run, because none of those rides on a
// field.
//
// The fix is the front end's, because the pristine vector is the front end's:
// interp only holds the one the caller is holding. What this file pins is the
// seam it relies on — that Runner.SetUp runs **after** the fields are copied
// and so can put the composed vector back. Without that ordering the front end
// could not correct it at all.
//
// Found only because bash's own shopt1.sub reaches it, and only once
// `compgen -A shopt` existed for that file to loop over (#4149).
func TestTheFreshShellsVectorCanBePutBackBySetUp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// `@(a)b` matches `ab` only where the extended patterns are on, so the
	// script reports which vector it was given.
	const body = "case ab in @(a)b) echo MATCHED;; esac\n"
	writeImage(t, dir, "noshebang.scr", []byte(body), 0o755)

	run := func(t *testing.T, callerHas bool, setUp func(*Runner)) string {
		t.Helper()
		var buf bytes.Buffer
		sem := imageSemantics()
		d := syntax.Core()
		d.ExtendedPattern = callerHas
		r := newTestRunner(t, &Runner{
			Stdout: &buf, Stderr: &buf,
			Semantics: &sem, Diagnostics: &Diagnostics{},
			Dialect: &d, Dir: dir, Name: "testsh", SetUp: setUp,
		})
		if setUp != nil {
			setUp(r)
		}
		f, err := syntax.Parse("./noshebang.scr\n", d)
		if err != nil {
			t.Fatal(err)
		}
		if _, rerr := r.Run(context.Background(), f); rerr != nil {
			t.Fatalf("run: %v", rerr)
		}
		return strings.TrimSpace(buf.String())
	}

	// `@(a)b` needs the extended patterns to *parse*, so the observable is
	// three-valued and each value says which vector the child was handed:
	// MATCHED for a child that has the flag, and a refusal to parse for one
	// that does not. The refusal is the point rather than a nuisance — a child
	// that had inherited the flag would have matched instead.
	//
	// With no SetUp the child takes the caller's vector, which is the behavior
	// that made the leak: a caller holding the moved flag hands it on.
	if got := run(t, true, nil); got != "MATCHED" {
		t.Errorf("with no SetUp the child read %q, want %q", got, "MATCHED")
	}
	// And a SetUp that puts the composed vector back wins, because it runs
	// after the fields are copied. This is the ordering the front end's fix
	// stands on; reverse it and the correction would be silently discarded.
	putBack := func(r *Runner) {
		d := syntax.Core()
		d.ExtendedPattern = false
		r.Dialect = &d
	}
	if got := run(t, true, putBack); strings.Contains(got, "MATCHED") || !strings.Contains(got, "unexpected") {
		t.Errorf("SetUp did not put the vector back: child read %q, want a refusal to parse", got)
	}
	// The other direction, so the row above cannot pass against a child that
	// simply never has the flag: a SetUp that composes it *on* hands it on.
	turnOn := func(r *Runner) {
		d := syntax.Core()
		d.ExtendedPattern = true
		r.Dialect = &d
	}
	if got := run(t, false, turnOn); got != "MATCHED" {
		t.Errorf("SetUp could not compose the flag on: child read %q, want %q", got, "MATCHED")
	}
}
