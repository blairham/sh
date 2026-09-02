// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A function carried to a child through the environment, which is the only
// way there is: an environment holds strings, so the source goes in and is
// parsed again at the other end. Which is why this needs the printer.
func TestAFunctionCanBeCarriedInTheEnvironment(t *testing.T) {
	t.Run("it reaches a child", func(t *testing.T) {
		out := carrying(t, `f(){ echo carried; }; export -f f; env | grep SH_FUNC_f`)
		if !strings.Contains(out, "SH_FUNC_f%%=() {  echo carried") {
			t.Errorf("env = %q, want the function written into it", out)
		}
	})

	t.Run("and a shell reads one back", func(t *testing.T) {
		// The other half, and the half that makes it worth anything: a name
		// the environment carries is a function here.
		out, st := run(t, `f`, func(r *Runner) {
			sem := CoreSemantics()
			r.Semantics = &sem
			r.SetFunctionExport("SH_FUNC_", "%%")
			r.Env = []string{"SH_FUNC_f%%=() { echo imported; }"}
		})
		if st != 0 || strings.TrimSpace(out) != "imported" {
			t.Errorf("out = %q status %d, want the environment's function run", out, st)
		}
	})

	t.Run("a dialect that carries nothing writes nothing", func(t *testing.T) {
		// Not an axis about the *option*, which is asked separately: this is
		// a shell with no name for the entry, so there is nowhere to put one.
		out, _ := run(t, `f(){ :; }; export -f f; env | grep -c FUNC_f || true`, func(r *Runner) {
			sem := CoreSemantics()
			sem.ExportCarriesFunctions = Yes
			r.Semantics = &sem
		})
		if strings.TrimSpace(out) != "0" {
			t.Errorf("env grep = %q, want nothing carried where there is no name for it", out)
		}
	})

	t.Run("a name that is not a function is refused", func(t *testing.T) {
		out, st := run(t, `export -f nope`, exporting)
		if st == 0 {
			t.Error("status 0, want it refused")
		}
		if !strings.Contains(out, "nope") {
			t.Errorf("out = %q, want the name in the refusal", out)
		}
	})

	t.Run("and one unset afterwards is not carried", func(t *testing.T) {
		// Exported, then gone. The child is told nothing rather than told
		// about a function this shell no longer has.
		// Through a filter, because `env` prints the *process* environment
		// too and this is only about the entry the shell adds.
		// Not through carrying: grep reports 1 when it counted nothing,
		// which is the answer this wants.
		out, _ := run(t, `f(){ :; }; export -f f; unset -f f; env | grep -c SH_FUNC_f`, exporting)
		if strings.TrimSpace(out) != "0" {
			t.Errorf("env = %q, want a function that no longer exists left out", out)
		}
	})
}

// The option is offered only where the dialect has it, and asked about only
// where there is one to decide.
func TestExportAsksAboutFOnlyWhenItIsThere(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refuses   bool
	}{
		{"no -f, so no question", `export A=1; echo "$A"`, false},
		{"an -f, so a question", `f(){ :; }; export -f f`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, func(r *Runner) {
				sem := CoreSemantics()
				r.Semantics = &sem
			})
			if got := strings.Contains(out, "no dialect was chosen"); got != tc.refuses {
				t.Errorf("refused = %v, want %v (out %q)", got, tc.refuses, out)
			}
		})
	}
}

func exporting(r *Runner) {
	sem := CoreSemantics()
	sem.ExportCarriesFunctions = Yes
	r.Semantics = &sem
	r.SetFunctionExport("SH_FUNC_", "%%")
}

func carrying(t *testing.T, src string) string {
	t.Helper()
	out, st := run(t, src, exporting)
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	return out
}
