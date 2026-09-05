// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// importedEnv is a Runner's environment with one name in it to reassign.
//
// Handed in rather than taken from the process, which is the point as much as
// the convenience: an embedder decides what a Runner inherited, and these
// cases are about what the Runner does with it.
func importedEnv() []string { return append(testPATH(), "IMPORTED=first") }

// TestAnImportedNameKeepsItsExportAttribute pins the rule that a variable
// which arrived in the environment is exported by having done so, and stays
// exported when it is assigned to.
//
// Unanimous across the panel and required by POSIX, which is why it is here
// rather than on the semantics vector. It read the other way for a while: the
// new value went into the runner's own table, nothing recorded that the name
// was exported, and the environment a child was given still carried the entry
// the shell was born with. The visible form of that was `PATH=/new:$PATH`
// followed by a build — the shell's own lookup found the new directory and
// every command it started was told the old PATH.
func TestAnImportedNameKeepsItsExportAttribute(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"assigned", `IMPORTED=second; env`},
		{"assigned in a function", `f() { IMPORTED=second; }; f; env`},
		{"assigned then read in a subshell", `IMPORTED=second; (env)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, func(r *Runner) { r.Env = importedEnv() })
			if !strings.Contains(out, "IMPORTED=second") {
				t.Errorf("out = %q, want the child told the new value", out)
			}
			if strings.Contains(out, "IMPORTED=first") {
				t.Errorf("out = %q, want the inherited value gone", out)
			}
		})
	}
}

// TestAnAssignmentSupersedesTheInheritedEntry: the child is handed the name
// once, not twice.
//
// Marking the name exported is only half the fix. The entry the shell was born
// with is still in the list, so without this the child receives both — and the
// stale one *first*, which is the one an exec'd program reads.
func TestAnAssignmentSupersedesTheInheritedEntry(t *testing.T) {
	out, _ := run(t, `IMPORTED=second; env | grep -c '^IMPORTED='`,
		func(r *Runner) { r.Env = importedEnv() })
	if strings.TrimSpace(out) != "1" {
		t.Errorf("out = %q, want the name in the child's environment exactly once", out)
	}
}

// TestUnsetTakesTheExportAttributeOff is the boundary the rule above must not
// cross, and it is unanimous too.
//
// `unset` is a removal rather than an assignment: the name goes back to being
// one this shell has never heard of, so assigning to it afterwards makes an
// ordinary shell variable and no child is told about it.
func TestUnsetTakesTheExportAttributeOff(t *testing.T) {
	out, _ := run(t, `unset IMPORTED; IMPORTED=second; env`,
		func(r *Runner) { r.Env = importedEnv() })
	if strings.Contains(out, "IMPORTED=") {
		t.Errorf("out = %q, want the child told nothing about an unset name", out)
	}
}

// TestTheExportAttributeCanBeTakenOffAndLeaveTheValue covers the explicit
// "no": the name stops reaching a child and goes on being readable here.
//
// The record has to be a tri-state for this to work — recorded on, recorded
// off, and never spoken about — because deleting the record puts the question
// back to the environment, which answers that the name is exported.
func TestTheExportAttributeCanBeTakenOffAndLeaveTheValue(t *testing.T) {
	out, _ := run(t, `export -n IMPORTED; echo "[$IMPORTED]"; env`, unexporting)
	if !strings.HasPrefix(out, "[first]\n") {
		t.Errorf("out = %q, want the shell still reading the value", out)
	}
	if strings.Contains(out, "IMPORTED=first") {
		t.Errorf("out = %q, want the child no longer told about it", out)
	}
}

// TestTakingTheAttributeOffSurvivesAReassignment: an assignment does not put
// back what an explicit refusal took away, which is the other half of the
// tri-state. Without it the name would be re-exported by being written to,
// since the environment it came from still names it.
func TestTakingTheAttributeOffSurvivesAReassignment(t *testing.T) {
	out, _ := run(t, `export -n IMPORTED; IMPORTED=second; echo "[$IMPORTED]"; env`, unexporting)
	if !strings.HasPrefix(out, "[second]\n") {
		t.Errorf("out = %q, want the shell reading the new value", out)
	}
	if strings.Contains(out, "IMPORTED=") {
		t.Errorf("out = %q, want no entry for it in the child's environment", out)
	}
}

// unexporting is a runner in a dialect that has `export -n`, with a name that
// arrived in the environment. The axis has to be answered for the two tests
// above to reach the letter at all: what `-n` does is not in question
// anywhere it exists, but whether the dialect has it is — see
// Semantics.ExportTakesTheAttributeOff.
func unexporting(r *Runner) {
	sem := CoreSemantics()
	sem.ExportTakesTheAttributeOff = Yes
	r.Semantics, r.Env = &sem, importedEnv()
}
