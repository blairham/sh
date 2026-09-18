// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A value that cannot aim a name reference is refused, and the refusal names
// **who made the write**. Two routes reach it with no builtin running and
// neither was named: an arithmetic command, where the construct names itself
// as it names its own arithmetic, and the descriptor a `{name}>` redirection
// stores, where the command word the redirection belongs to speaks.
//
// The second route was short a whole sentence as well, and the sentence is
// written for *any* refused descriptor store rather than for this one — a
// frozen name gets it too. And a refused store is the **redirection** failing,
// so the command does not run (#3491).

// The arithmetic command names itself, exactly as it does for its own
// arithmetic — measured 2026-09-17, bash 5.3.20 writes
// “((: `1': not a valid identifier“ where the same write through a builtin
// is “read: `1': …“ and a bare assignment is unprefixed.
func TestABadNamerefAimInsideAnArithmeticCommandNamesTheConstruct(t *testing.T) {
	out, _ := aimSpeakerRunWith(t, Diagnostics{ArithErrorNamesTheConstruct: true},
		"typeset -n r\n"+`(( r = 1 )); echo "st=$?"`)
	if !strings.Contains(out, "((: `1': not a valid identifier") {
		t.Errorf("out %q does not name the construct", out)
	}
	// And the bare assignment still names nobody, which is what makes the
	// line above a subject rather than a prefix on everything.
	out, _ = aimSpeakerRunWith(t, Diagnostics{ArithErrorNamesTheConstruct: true},
		"typeset -n q\nq=/")
	if strings.Contains(out, "((: ") {
		t.Errorf("out %q names a construct that is not running", out)
	}
	if !strings.Contains(out, "`/': not a valid identifier") {
		t.Errorf("out %q does not write the refusal", out)
	}
}

// The descriptor a `{name}>` stores is refused under the **command word** the
// redirection belongs to, and a compound command has none.
func TestARefusedDescriptorStoreNamesTheCommandItsRedirectionBelongsTo(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a builtin", "typeset -n s\ntrue {s}>/dev/null", "true: `"},
		{"a command word", "typeset -n s\n/nonexistent/x {s}>/dev/null", "/nonexistent/x: `"},
		{"a compound command", "typeset -n s\n{ echo z; } {s}>/dev/null", "`"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := aimSpeakerRun(t, c.src)
			if !strings.Contains(out, c.want+"10': not a valid identifier") {
				t.Errorf("out %q does not name the writer as %q", out, c.want)
			}
		})
	}
	// The group's own refusal carries no name at all, which is the row that
	// keeps the two apart — the shell's own location is all that precedes it.
	out, _ := aimSpeakerRun(t, "typeset -n s\n{ echo z; } {s}>/dev/null")
	if !strings.HasPrefix(out, "sh: `10': not a valid identifier\n") {
		t.Errorf("out %q names a writer a compound command does not have", out)
	}
}

// The **second** sentence, which follows whatever the store itself said and is
// written for any refused store rather than for one of them.
func TestARefusedDescriptorStoreWritesTheSecondSentence(t *testing.T) {
	dg := Diagnostics{CannotAssignFdToVariable: "FD %[1]s"}
	for _, c := range []struct{ name, src string }{
		// `true` rather than `exec`, whose *own* redirection failure is
		// fatal under the standard's preset — a question of its own, and one
		// this row is not about.
		{"a bad aim", "typeset -n s\ntrue {s}>/dev/null"},
		{"a frozen name", "s=1; readonly s\ntrue {s}>/dev/null"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := aimSpeakerRunWith(t, dg, c.src+"\necho \"st=$?\"\necho after")
			if !strings.Contains(out, "FD s") {
				t.Errorf("out %q does not write the second sentence", out)
			}
			if !strings.Contains(out, "st=1") {
				t.Errorf("out %q does not leave the redirection's failure behind", out)
			}
			if !strings.Contains(out, "after") {
				t.Errorf("out %q gave up more than the command", out)
			}
		})
	}
	// Empty writes nothing, which is every dialect with no such sentence —
	// and the redirection still fails, because that half is not a wording.
	out, _ := aimSpeakerRun(t, "typeset -n s\n{ echo z; } {s}>/dev/null\necho \"st=$?\"")
	if strings.Contains(out, "cannot assign") {
		t.Errorf("out %q writes a sentence the dialect does not have", out)
	}
	if strings.Contains(out, "z") {
		t.Errorf("out %q ran a command whose redirection failed", out)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("out %q does not leave the redirection's failure behind", out)
	}
}

// A store that *succeeds* still stores, which is what keeps the failure path
// above a check rather than a wall.
func TestADescriptorStoreThatSucceedsStillRunsTheCommand(t *testing.T) {
	out, _ := aimSpeakerRun(t, `{ echo z; } {s}>/dev/null; echo "st=$? s=[$s]"`)
	if !strings.Contains(out, "st=0 s=[10]") {
		t.Errorf("out %q, want the descriptor stored and the command run", out)
	}
}

func aimSpeakerRun(t *testing.T, src string) (string, int) {
	t.Helper()
	return aimSpeakerRunWith(t, Diagnostics{}, src)
}

// aimSpeakerRunWith needs the two constructs these cases are about — a name
// reference and a `{name}>` redirection — and nothing else about a dialect.
func aimSpeakerRunWith(t *testing.T, dg Diagnostics, src string) (string, int) {
	t.Helper()
	sem := namerefAimSemantics()
	// A frozen name's refusal is not fatal here, so a row about the sentence
	// the redirection adds can see the line behind it. What that refusal ends
	// is its own axis and not this one.
	sem.ReadonlyReassignmentFatal = No
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.FdVariableRedirections = true
	}, func(r *Runner) { r.Semantics = &sem; r.Diagnostics = &dg })
}
