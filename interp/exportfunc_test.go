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
	// With no arrangement asked for, which is what a runner told nothing
	// gets: the core has no layout of its own and writes the body on one
	// line. What shape it takes is the dialect's to say.
	t.Run("it reaches a child", func(t *testing.T) {
		out := carrying(t, `f(){ echo carried; }; export -f f; env | grep SH_FUNC_f`)
		if !strings.Contains(out, "SH_FUNC_f%%=() { echo carried; }") {
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

	t.Run("unset -f removes the function itself", func(t *testing.T) {
		// The option was read and then ignored, so a function survived being
		// unset and went on answering to its name.
		out, st := run(t, `f(){ echo F; }; unset -f f; f`, exporting)
		if st == 0 || strings.Contains(out, "F") {
			t.Errorf("out = %q status %d, want the function gone", out, st)
		}
	})

	t.Run("and unsetting it forgets that it was exported", func(t *testing.T) {
		// Measured: a function defined again after `unset -f` is not
		// carried, so the export goes with the function rather than sticking
		// to the name.
		out, _ := run(t, `f(){ :; }; export -f f; unset -f f; f(){ :; }; env | grep -c SH_FUNC_f`, exporting)
		if strings.TrimSpace(out) != "0" {
			t.Errorf("env grep = %q, want the export forgotten with the function", out)
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

// A dialect that knows the letter and will not do it says something different
// from one that has never heard of it — measured twice, because the first
// reading looked like a wording per builtin and it is per option.
func TestADialectCanRefuseTheFunctionOptionInItsOwnWords(t *testing.T) {
	out, st := run(t, `export -f f`, func(r *Runner) {
		sem := CoreSemantics()
		sem.ExportCarriesFunctions = No
		dg := Diagnostics{ExportFunctionOptionRefused: "invalid option(s)"}
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	if !strings.Contains(out, "invalid option(s)") {
		t.Errorf("out = %q, want the dialect's own refusal", out)
	}
	if strings.Contains(out, "-f") {
		t.Errorf("out = %q, want the letter left unnamed, which is the whole difference", out)
	}
	if st == 0 {
		t.Error("status 0, want it refused")
	}
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

// `-n` is offered only where the dialect has it, which is the same shape `-f`
// has above and for the same reason: what the letter means is settled
// everywhere it exists — the name stays set and stops reaching a child — and
// only its availability splits the panel. So there is no wording to carry,
// and a dialect without it sends `-n` down the ordinary unknown-option path
// to collect its own refusal, fatal or not as that dialect says (#516).
func TestExportOffersTheOffOptionOnlyWhereTheDialectHasIt(t *testing.T) {
	t.Run("a dialect that has it", func(t *testing.T) {
		// The script's own status is grep's, which is 1 for a count of
		// nought — the interesting number is `export`'s, which the script
		// prints.
		out, _ := run(t, `V=1; export V; export -n V; echo "st=$?"; env | grep -c "^V="`,
			func(r *Runner) {
				sem := CoreSemantics()
				sem.ExportTakesTheAttributeOff = Yes
				r.Semantics = &sem
			})
		if out != "st=0\n0\n" {
			t.Errorf("out = %q, want the letter taken and the child told nothing", out)
		}
	})
	t.Run("a dialect that does not", func(t *testing.T) {
		out, _ := run(t, `V=1; export V; export -n V; echo "st=$?"; env | grep -c "^V="`,
			func(r *Runner) {
				sem := CoreSemantics()
				sem.ExportTakesTheAttributeOff = No
				r.Semantics = &sem
			})
		if !strings.Contains(out, "-n: invalid option") {
			t.Errorf("out = %q, want the ordinary unknown-option refusal", out)
		}
		if strings.Contains(out, "st=0") {
			t.Errorf("out = %q, want `export` to have failed", out)
		}
		if !strings.Contains(out, "\n1\n") {
			t.Errorf("out = %q, want the name still exported — a refused option changes nothing", out)
		}
	})
}

// The standard's own answer, which the preset takes from the text rather
// than from a vote: POSIX spells `export` with `-p` and nothing else, so a
// POSIX shell has no `-n`. Without it the preset would refuse `export -n` as
// an axis nobody chose rather than as the unknown option the standard makes
// it.
func TestThePosixPresetHasNoOffOption(t *testing.T) {
	if got := PosixSemantics().ExportTakesTheAttributeOff; got != No {
		t.Errorf("PosixSemantics().ExportTakesTheAttributeOff = %v, want No", got)
	}
	out, _ := run(t, `export -n V; echo "st=$?"`, func(r *Runner) {
		sem := PosixSemantics()
		// Not fatal, so what is read is the refusal rather than a dead
		// script — the fatality is a dialect's answer and not this one's.
		sem.BadOptionToSpecialBuiltinFatal = No
		r.Semantics = &sem
	})
	if !strings.Contains(out, "-n: invalid option") {
		t.Errorf("out = %q, want the unknown-option refusal", out)
	}
	if strings.Contains(out, "no dialect was chosen") {
		t.Errorf("out = %q, want an answer rather than a refusal to answer", out)
	}
}

// And asked about only where there is an `-n` to decide, so a dialect that
// has not chosen can still export.
func TestExportAsksAboutTheOffOptionOnlyWhenItIsThere(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refuses   bool
	}{
		{"no -n, so no question", `export A=1; echo "$A"`, false},
		{"an -n, so a question", `export -n A`, true},
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
