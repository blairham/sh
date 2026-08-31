// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	. "github.com/blairham/sh/interp"
)

func TestNounset(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		wantOut   string
		stops     bool
	}{
		{"an unset variable stops the script", `set -u; echo "[$NOPE]"; echo after`, "", true},
		{"off by default", `echo "[$NOPE]"; echo after`, "[]\nafter\n", false},
		{"set +u turns it back off", `set -u; set +u; echo "[$NOPE]"; echo after`, "[]\nafter\n", false},

		// The exemptions, all unanimous.
		{"a default supplies a value", `set -u; echo "[${NOPE:-d}]"; echo after`, "[d]\nafter\n", false},
		{"and the other default form", `set -u; echo "[${NOPE-d}]"; echo after`, "[d]\nafter\n", false},
		{"the alternate form asks rather than uses", `set -u; echo "[${NOPE+a}]"; echo after`, "[]\nafter\n", false},
		{"set but empty is not unset", `set -u; E=; echo "[$E]"; echo after`, "[]\nafter\n", false},

		// Not obvious, and often got wrong: no parameters is not unset.
		{"$@ with none is quiet", `set -u; echo "[$@]"; echo after`, "[]\nafter\n", false},
		{"$* with none is quiet", `set -u; echo "[$*]"; echo after`, "[]\nafter\n", false},

		// And where it still fires.
		{"the length of an unset one", `set -u; echo "[${#NOPE}]"; echo after`, "", true},
		{"an assignment from an unset one", `set -u; x=$NOPE; echo after`, "", true},
		{"inside a function too", `set -u; f() { echo "[$NOPE]"; }; f; echo after`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := run(t, tc.src, withSem(bash.Semantics()))
			if tc.stops {
				if strings.Contains(got, "after") {
					t.Errorf("the script continued: %q", got)
				}
				if st == 0 {
					t.Error("status should not be 0")
				}
				return
			}
			if got != tc.wantOut || st != 0 {
				t.Errorf("got %q/%d, want %q/0", got, st, tc.wantOut)
			}
		})
	}
}

// TestUnsetPositionalIsAnAxis is ksh93's alone: there an argument the script
// was not given expands to nothing, where the other three stop. Quiet either
// way, which is what makes it worth an axis rather than a preference.
func TestUnsetPositionalIsAnAxis(t *testing.T) {
	const src = `set -u; echo "[$1]"; echo after`
	if got, st := run(t, src, withSem(ksh.Semantics())); got != "[]\nafter\n" || st != 0 {
		t.Errorf("ksh93: got %q/%d, want %q/0", got, st, "[]\nafter\n")
	}
	for _, tc := range []struct {
		name string
		sem  Semantics
	}{{"dash", dash.Semantics()}, {"bash", bash.Semantics()}, {"zsh", zsh.Semantics()}} {
		got, st := run(t, src, withSem(tc.sem))
		if strings.Contains(got, "after") || st == 0 {
			t.Errorf("%s: should have stopped, got %q/%d", tc.name, got, st)
		}
	}
	// A positional in range is fine everywhere.
	for _, sem := range []Semantics{ksh.Semantics(), dash.Semantics()} {
		if got, _ := run(t, `set -u; set -- a; echo "[$1]"`, withSem(sem)); got != "[a]\n" {
			t.Errorf("in range: got %q", got)
		}
	}
}

// TestUnsetPositionalTakesADefault is the bug `set -u` turned up: an
// out-of-range positional was reported as *set*, so the plain default form
// never fired for it.
func TestUnsetPositionalTakesADefault(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo "[${1-default}]"`, "[default]\n"},
		{`echo "[${1:-default}]"`, "[default]\n"},
		{`set -- a; echo "[${2-default}]"`, "[default]\n"},
		{`set -- a b; echo "[${2-default}]"`, "[b]\n"},
	} {
		if got, _ := run(t, tc.src, withSem(bash.Semantics())); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestUnboundWordingIsTheDialects(t *testing.T) {
	// bash calls it unbound where the other three call it not set. The
	// wording lives in Diagnostics, so withSem alone cannot see it — which
	// is worth a helper rather than a comment, since every wording test
	// after this one needs both halves of the dialect.
	withDialect := func(sem Semantics, diag Diagnostics) func(*Runner) {
		return func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &diag }
	}
	if got, _ := run(t, `set -u; echo "$NOPE"`, withDialect(bash.Semantics(), bash.Diagnostics())); !strings.Contains(got, "unbound variable") {
		t.Errorf("bash: got %q", got)
	}
	if got, _ := run(t, `set -u; echo "$NOPE"`, withDialect(dash.Semantics(), dash.Diagnostics())); !strings.Contains(got, "parameter not set") {
		t.Errorf("dash: got %q", got)
	}
}
