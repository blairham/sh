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

func setRun(t *testing.T, dir string, tweak func(*Semantics), src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.PipefailOption = Yes
	sem.SetFTurnsOffGlobbing = Yes
	sem.ErrexitSeesPipefailFailure = Yes
	if tweak != nil {
		tweak(&sem)
	}
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Dir: dir, Name: "testsh"})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// `-o` is nearly always the last letter of a bundle rather than a word of its
// own: `set -euo pipefail` is the line at the top of a great many scripts, and
// it was refused outright.
func TestOAtTheEndOfABundle(t *testing.T) {
	dir := t.TempDir()
	// The named option takes effect. Without `-e` here: with it the script
	// stops at the failing pipeline and never reaches the echo, which is
	// itself the behavior TestErrexitAndAPipefailOnlyFailure is about.
	out, _ := setRun(t, dir, nil, "set -uo pipefail\n(exit 3) | true\necho p=$?")
	if !strings.Contains(out, "p=3") {
		t.Errorf("said %q, want the named option applied", out)
	}
	// ...and so do the letters before it, which is the half a fix that only
	// looked for the name would drop.
	out, _ = setRun(t, dir, nil, "set -euo pipefail\necho ${NOPE}\necho unreached")
	if strings.Contains(out, "unreached") {
		t.Errorf("said %q, want nounset from the same bundle", out)
	}
	out, _ = setRun(t, dir, nil, "set -euo pipefail\nfalse\necho unreached")
	if strings.Contains(out, "unreached") {
		t.Errorf("said %q, want errexit from the same bundle", out)
	}
	// And `+` turns the named one off again.
	out, _ = setRun(t, dir, nil, "set -o pipefail\nset +o pipefail\n(exit 3) | true\necho p=$?")
	if !strings.Contains(out, "p=0") {
		t.Errorf("said %q, want pipefail off", out)
	}
	out, _ = setRun(t, dir, nil, "set -o pipefail\nset +xo pipefail\n(exit 3) | true\necho p=$?")
	if !strings.Contains(out, "p=0") {
		t.Errorf("said %q, want pipefail off from a bundle too", out)
	}
}

// A bundle ending in `o` with nothing after it has no name to read.
func TestABundleEndingInOListsTheOptions(t *testing.T) {
	// `set -eo` with nothing after it applies the letters and then lists —
	// measured in bash, dash and zsh alike; the old refusal ("-o needs an
	// option name") was ours alone.
	out, st := run(t, `set -eo; echo reached`, nil)
	if st != 0 || !strings.Contains(out, "errexit") || !strings.Contains(out, "reached") {
		t.Errorf("said %q status %d, want the listing and the script carrying on", out, st)
	}
	if !strings.Contains(out, "errexit         on") && !strings.Contains(out, "errexit        on") {
		t.Errorf("said %q, want the letter applied before the listing", out)
	}
}

// A letter this shell does not have is still refused, and refused before the
// name is read rather than after — otherwise `set -Zo pipefail` would set
// pipefail on its way to complaining.
func TestABadLetterInABundleIsRefusedFirst(t *testing.T) {
	dir := t.TempDir()
	// `set`'s own status, not the script's — the echo after it succeeds and
	// would report 0 whatever `set` did.
	out, _ := setRun(t, dir, nil, "set -Zo pipefail\necho s=$?\n(exit 3) | true\necho p=$?")
	if !strings.Contains(out, "s=2") || !strings.Contains(out, "set: -Z: invalid option") {
		t.Errorf("said %q, want the letter refused with status 2", out)
	}
	if !strings.Contains(out, "p=0") {
		t.Errorf("said %q, want pipefail never to have been set", out)
	}
}

// `set -e` stops for a failure that only pipefail produced in two of the three
// shells that have the option. An ordinary failing pipeline stops all of them,
// so the axis is only about the failure the option adds.
func TestErrexitAndAPipefailOnlyFailure(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name    string
		answer  Answer
		reached bool
	}{
		{"bash and zsh stop", Yes, false},
		{"ksh93 runs on", No, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			tweak := func(s *Semantics) { s.ErrexitSeesPipefailFailure = c.answer }
			out, _ := setRun(t, dir, tweak, "set -eo pipefail\nfalse | true\necho reached")
			if got := strings.Contains(out, "reached"); got != c.reached {
				t.Errorf("said %q, want reached=%v", out, c.reached)
			}
			// The ordinary case is unanimous and must not move with it.
			out, _ = setRun(t, dir, tweak, "set -eo pipefail\ntrue | false\necho unreached")
			if strings.Contains(out, "unreached") {
				t.Errorf("ordinary failure: said %q, want it to stop either way", out)
			}
			// Nor may a later simple command inherit the exemption.
			out, _ = setRun(t, dir, tweak, "set -eo pipefail\nfalse | true\nfalse\necho unreached")
			if strings.Contains(out, "unreached") {
				t.Errorf("stale exemption: said %q, want the plain false to stop", out)
			}
		})
	}
}
