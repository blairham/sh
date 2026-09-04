// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `read` skipped every leading `-` word, whatever it was. Two of those are
// worth naming: `-s` would have echoed what was meant to be hidden, and `-n 1`
// would have read a whole line. Both silently.

// TestReadKeepsItsOwnOption is the control: `-r` is the one option this shell
// implements, and it has to keep working now that the others are refused.
//
// A backslash at the end of a line is where it shows. A backslash *inside* a
// line is not: `read v` on `a\b` gives `ab` in the shell this follows and
// `a\b` here, because unescaping has to happen while the fields are split —
// an escaped space does not end a field there — and this splits first. A
// separate gap, measured, and not one `-r` can hide.
func TestReadKeepsItsOwnOption(t *testing.T) {
	joined, _ := run(t, "printf 'a\\\\\nb\n' | { read v; echo \"[$v]\"; }", nil)
	if !strings.Contains(joined, "[ab]") {
		t.Errorf("read: got %q, want the line continued", joined)
	}
	kept, _ := run(t, "printf 'a\\\\\nb\n' | { read -r v; echo \"[$v]\"; }", nil)
	if !strings.Contains(kept, `[a\]`) {
		t.Errorf("read -r: got %q, want the backslash kept and the line ended", kept)
	}
}

// TestReadRefusesAnOptionItDoesNotHave rather than skipping it, which is what
// makes the difference between a shell that says it cannot do something and
// one that quietly does something else.
func TestReadRefusesAnOptionItDoesNotHave(t *testing.T) {
	out, _ := run(t, `read -q v </dev/null`, func(r *Runner) {
		s := *r.Semantics
		s.BadOptionToSpecialBuiltinFatal = No
		r.Semantics = &s
		dg := Diagnostics{BuiltinBadOption: "read: %[2]s: bad"}
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "read: -q: bad") {
		t.Errorf("got %q, want the option refused", out)
	}
}

// TestReadSaysWhenAnOptionIsMerelyMissing, which is the answer for the ones
// the dialect really has — `-s` above all.
func TestReadSaysWhenAnOptionIsMerelyMissing(t *testing.T) {
	out, _ := run(t, `read -s v </dev/null`, func(r *Runner) {
		s := *r.Semantics
		s.BadOptionToSpecialBuiltinFatal = No
		r.Semantics = &s
		dg := Diagnostics{
			BuiltinBadOption:           "read: %[2]s: bad",
			UnimplementedOptionLetters: map[string]string{"read": "s"},
		}
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "not implemented yet") {
		t.Errorf("got %q, want it said to be missing rather than unknown", out)
	}
}

// TestReadSplitsAClusteredBundle: `-rr` is two options in one word, both of
// them the letter this shell implements. Reading the word whole refused it —
// the bundle was not `-r`, so it went to the refusal path with an option the
// builtin has inside it (#347).
func TestReadSplitsAClusteredBundle(t *testing.T) {
	out, _ := run(t, "printf 'x\\\\\ny\n' | { read -rr v; echo \"[$v]\"; }", nil)
	if !strings.Contains(out, `[x\]`) {
		t.Errorf("read -rr: got %q, want the bundle read as two -r flags", out)
	}
}

// TestAMissingOptionInABundleIsNamedAlone: the half of #347 that shows while
// `-a` is unimplemented (#321). `read -ra` is `-r -a`, and the complaint is
// about `-a` — the missing letter — not about a word `-ra` that no shell
// would refuse.
func TestAMissingOptionInABundleIsNamedAlone(t *testing.T) {
	out, _ := run(t, `read -ra arr </dev/null`, func(r *Runner) {
		s := *r.Semantics
		s.BadOptionToSpecialBuiltinFatal = No
		r.Semantics = &s
		dg := Diagnostics{
			BuiltinBadOption:           "read: %[2]s: bad",
			UnimplementedOptionLetters: map[string]string{"read": "a"},
		}
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "-a is not implemented yet") {
		t.Errorf("got %q, want -a said to be missing", out)
	}
	if strings.Contains(out, "-ra") {
		t.Errorf("got %q, want the letter named without the bundle", out)
	}
}

// TestAnUnknownOptionInABundleIsNamedAlone is the same rule on the refusal
// path: the shells walk the bundle and stop at the letter they cannot use, so
// the complaint names that letter and not the word it rode in on.
func TestAnUnknownOptionInABundleIsNamedAlone(t *testing.T) {
	out, _ := run(t, `read -rq v </dev/null`, func(r *Runner) {
		s := *r.Semantics
		s.BadOptionToSpecialBuiltinFatal = No
		r.Semantics = &s
		dg := Diagnostics{BuiltinBadOption: "read: %[2]s: bad"}
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "read: -q: bad") {
		t.Errorf("got %q, want the offending letter named alone", out)
	}
}

// TestDashDashEndsReadsOptions, so a variable named like an option can still
// be read into.
func TestDashDashEndsReadsOptions(t *testing.T) {
	out, _ := run(t, `printf 'x\n' | { read -- v; echo "[$v]"; }`, nil)
	if !strings.Contains(out, "[x]") {
		t.Errorf("got %q, want -- taken as the end of the options", out)
	}
}
