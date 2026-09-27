// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The `..` axis read under a **physical** resolution — see
// Semantics.CdCancelsADotDot, which carries the panel, and
// Runner.physicalPathCancelingIntoTheDirectoryHeld for the grid.
//
// Three of the four columns resolve the whole path with every `..` in place
// there; one cancels a `..` that reaches past the operand and into the
// directory the shell is **logically** in, and resolves only what is left. It
// is the same reading the `-L` route is keyed on, which is why it is the same
// axis rather than a second one.
//
// **The two kinds of `..` interleave**, which is what rules out a two-part
// split: `deep/../..` has one of each, and canceling the logical ones first
// would leave the physical walk somewhere the second `..` cannot be read
// from. The row is in the table below, and it is also what rules out "a
// leading `..`" as the noun — the answer is decided per component rather than
// by what the operand starts with.
func cdWithinRun(t *testing.T, a CdDotDotCancellationPolicy, root, src string) (string, string) {
	t.Helper()
	out, errs := &strings.Builder{}, &strings.Builder{}
	sem := PosixSemantics()
	sem.CdCancelsADotDot = a
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Dir: root,
		Stdout: out, Stderr: errs,
	})
	runCd(t, r, src+"\n")
	return strings.TrimSpace(out.String()), errs.String()
}

const cdRefused = "\x00refused"

func TestWhereAPhysicalDotDotReachesPastTheOperandIsAnAxis(t *testing.T) {
	for _, c := range []struct {
		name, src           string
		within, everyDotDot string
	}{
		{
			// The row the axis is for: the `..` reaches past the operand
			// and into `$PWD`, whose last component is the link's own name.
			"a bare .. out of a logically reached link",
			"cd sub/fake && cd -P ..", "sub", "",
		},
		{
			// The same row with a `.` in front of it, which is what says
			// the rule is not keyed on what the operand starts with.
			"a . before it changes nothing",
			"cd sub/fake && cd -P ./..", "sub", "",
		},
		{
			// A cancellation that builds a path nothing is at. The reading
			// that resolves would have followed the link out and arrived.
			"a canceled .. with a component after it",
			"cd sub/fake && cd -P ../real", cdRefused, "real",
		},
		{
			// The sharpest row, and the one that needs both readings live
			// in one operand: the first `..` cancels the link's own name
			// logically, putting the walk where a `fake` really is, and the
			// second resolves it physically. The other reading never takes
			// the first step and refuses the row outright.
			"one operand that needs both readings",
			"cd sub/fake && cd -P ../fake/..", "", cdRefused,
		},
		{
			// One `..` of each kind, and the row a two-part split gets
			// wrong. Both readings arrive in the same place, which is what
			// makes it a control here and a discriminator in the code.
			"a .. inside the operand and a .. past it",
			"cd sub/fake && cd -P deep/../..", "", "",
		},
		{
			// Every `..` inside the operand: unanimous, and the row that
			// says this axis has not taken over `-P` altogether.
			"a .. straight after a link, inside the operand",
			"cd -P sub/fake/..", "", "",
		},
		{
			"a .. one component below a link",
			"cd -P sub/fake/deep/..", "real", "real",
		},
		{
			// The row that looks like agreement and is not evidence: the
			// two readings coincide one level down.
			"a bare .. one level below the link",
			"cd sub/fake/deep && cd -P ..", "real", "real",
		},
		{
			// A `..` over a component that is not there is refused under
			// every reading, because it is the operand's own.
			"a .. over a component that is not there",
			"cd -P nosuch/..", cdRefused, cdRefused,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, r := range []struct {
				a    CdDotDotCancellationPolicy
				want string
			}{
				{CdDotDotLooksWithinTheOperand, c.within},
				{CdDotDotCanceledUnseen, c.everyDotDot},
				{CdDotDotLooksAtEveryCanceledComponent, c.everyDotDot},
			} {
				root := dotDotTree(t)
				got, errs := cdWithinRun(t, r.a, root, c.src+" && pwd")
				if r.want == cdRefused {
					if errs == "" {
						t.Errorf("%v: %s arrived at %q, want a refusal", r.a, c.src, got)
					}
					continue
				}
				if errs != "" {
					t.Errorf("%v: %s said %q, want it to arrive", r.a, c.src, errs)
					continue
				}
				want := root
				if r.want != "" {
					want = filepath.Join(root, r.want)
				}
				if got != want {
					t.Errorf("%v: %s left %q, want %q", r.a, c.src, got, want)
				}
			}
		})
	}
}

// What the refusal names, which is the other half of the reading being
// visible: the path the cancellation **built**, rather than the operand. The
// operand is not where this reading looked.
//
// And the control beside it: a failure in the operand's own components is
// blamed as written, because there the cancellation never got as far as
// rebuilding anything.
func TestACanceledPhysicalCdBlamesThePathItBuilt(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the canceled path", "cd sub/fake && cd -P ../real", "sub/real"},
		{"and a component of the operand's own", "cd -P nosuch/..", "nosuch/.."},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := dotDotTree(t)
			_, errs := cdWithinRun(t, CdDotDotLooksWithinTheOperand, root, c.src)
			if !strings.Contains(errs, c.want) {
				t.Errorf("%s said %q, want it to name %q", c.src, errs, c.want)
			}
		})
	}
}

// A cancellation looks the built name **up**; it does not fall back on the
// descriptor this Runner is holding.
//
// The two part where the name has stopped leading where it did: with the
// shell in `d/s`, `d` renamed away and a fresh `d` made at the old name, this
// reading arrives in the new one. A `cd` that asked its descriptor would
// arrive in the renamed directory instead, which is a different place with
// different contents — so the row is read from what is *in* the directory
// rather than from its name.
func TestACanceledPhysicalCdLooksTheBuiltNameUp(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "d", "s"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, errs := &strings.Builder{}, &strings.Builder{}
	sem := PosixSemantics()
	sem.CdCancelsADotDot = CdDotDotLooksWithinTheOperand
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Dir: filepath.Join(root, "d", "s"),
		Stdout: out, Stderr: errs,
	})
	runCd(t, r, "cd -P .\n")
	if err := os.Rename(filepath.Join(root, "d"), filepath.Join(root, "e")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "d", "mark"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	runCd(t, r, "cd -P .. && pwd\n")
	if errs.Len() != 0 {
		t.Fatalf("stderr = %q", errs.String())
	}
	if got, want := strings.TrimSpace(out.String()), filepath.Join(root, "d"); got != want {
		t.Fatalf("left %q, want %q", got, want)
	}
	// The contents rather than the name, because both directories can be
	// called `d` and only one of them holds this.
	if _, err := os.Stat(filepath.Join(r.Dir, "mark")); err != nil {
		t.Errorf("the shell is not in the directory the name now leads to: %v", err)
	}
}
