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

// `cd -s`, the fourth option letter and the second that belongs to one shell:
// it refuses an operand that crosses a symbolic link. Measured 2026-09-12 on
// zsh 5.9.2, in a directory holding `real/sub` and a `link` pointing at
// `real` — `cd -s real` moves, `cd -s link` and `cd -s link/sub` are both
// `not a directory` at 1 and stay where they were.
//
// The axis is CdHasSymlinkFreeOption; the shell it belongs to is named in
// dialect/zsh and never here.

// cdSymlinkFreeRunner is a runner with the letter answered yes and the
// unknown-letter refusal turned off, which is the pair that reaches the
// behavior at all.
func cdSymlinkFreeRunner(t *testing.T, dir string, out, errs *strings.Builder) *Runner {
	t.Helper()
	sem := PosixSemantics()
	sem.CdHasSymlinkFreeOption = Yes
	sem.CdRefusesUnknownOption = No
	return newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Dir: dir,
		Stdout: out, Stderr: errs,
	})
}

// What the letter refuses, and what it does not.
func TestCdSymlinkFreeRefusesALinkedOperand(t *testing.T) {
	for _, c := range []struct {
		name    string
		cmd     string
		refused bool
		// viaLink starts the shell in a directory reached through a link,
		// which is the one thing backing out of a link asks about.
		viaLink bool
	}{
		{"a path with no link in it", "cd -s real", false, false},
		{"the link itself", "cd -s link", true, false},
		{"a link partway along", "cd -s link/sub", true, false},
		// Backed out of, a link is answered against the path the shell
		// stands in: measured 2026-10-03 on zsh 5.9.2, it moves from a
		// directory with no link above it and is refused from one reached
		// through a link. Backing out of an ordinary directory never asks.
		{"a link canceled by a later ..", "cd -s link/../real", false, false},
		{"the same from a linked directory", "cd -s link/../real", true, true},
		{"an ordinary directory from a linked one", "cd -s real/sub/..", false, true},
		// `.` and `..` are not links, so a path that only goes through them
		// is accepted — the walk lstats each component where it stands
		// rather than cleaning the path first.
		{"a dot component", "cd -s ./real", false, false},
		{"a parent component", "cd -s real/sub/..", false, false},
		{"a round trip through the parent", "cd -s real/../real", false, false},
		// The letter is a letter: bundled and repeated it still reads as one.
		{"bundled with -P", "cd -sP link", true, false},
		{"repeated", "cd -s -s link", true, false},
		{"bundled with -P, other order", "cd -Ps real", false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, dir := cdTree(t)
			if c.viaLink {
				via := filepath.Join(t.TempDir(), "via")
				if err := os.Symlink(dir, via); err != nil {
					t.Fatal(err)
				}
				dir = via
			}
			out, errs := &strings.Builder{}, &strings.Builder{}
			r := cdSymlinkFreeRunner(t, dir, out, errs)
			runCd(t, r, c.cmd+"\necho st=$?\n")
			refused := !strings.Contains(out.String(), "st=0\n")
			if refused != c.refused {
				t.Fatalf("%s: out %q err %q, want refused=%v",
					c.cmd, out.String(), errs.String(), c.refused)
			}
			// Asserted as arrival and not only as status: a `cd` that
			// reported and moved anyway, or that refused and moved, passes
			// every check on the number alone.
			if c.refused {
				if r.Dir != dir {
					t.Errorf("%s refused and still moved to %q", c.cmd, r.Dir)
				}
				if errs.Len() == 0 {
					t.Errorf("%s refused in silence, want a diagnostic", c.cmd)
				}
				return
			}
			if r.Dir == dir {
				t.Errorf("%s did not move", c.cmd)
			}
			if errs.Len() != 0 {
				t.Errorf("%s said %q, want nothing", c.cmd, errs.String())
			}
		})
	}
}

// The walk starts where the shell already is and looks only at the operand's
// own components, which is the measurement that separates this from "refuse
// to be anywhere reached through a link": having moved *into* the link, a
// further `cd -s sub` moves.
func TestCdSymlinkFreeLooksAtTheOperandAndNotThePlace(t *testing.T) {
	dir, _ := cdTree(t)
	out, errs := &strings.Builder{}, &strings.Builder{}
	r := cdSymlinkFreeRunner(t, dir, out, errs)
	runCd(t, r, "cd link\necho first=$?\ncd -s sub\necho st=$?\n")
	if errs.Len() != 0 {
		t.Fatalf("said %q, want nothing", errs.String())
	}
	if got := out.String(); !strings.Contains(got, "first=0\n") ||
		!strings.Contains(got, "st=0\n") {
		t.Fatalf("printed %q, want both moves to succeed", got)
	}
	if !strings.HasSuffix(r.Dir, "link/sub") {
		t.Errorf("landed in %q, want the sub under the link", r.Dir)
	}
}

// A component that is not there ends the walk with no refusal of its own, so
// the ordinary missing-directory failure is what reports. Measured: `cd -s
// nosuch` is `no such file or directory` and not `not a directory`.
func TestCdSymlinkFreeLeavesAMissingPathToTheOrdinaryFailure(t *testing.T) {
	dir, _ := cdTree(t)
	out, errs := &strings.Builder{}, &strings.Builder{}
	r := cdSymlinkFreeRunner(t, dir, out, errs)
	runCd(t, r, "cd -s real/nosuch\necho st=$?\n")
	if strings.Contains(out.String(), "st=0\n") {
		t.Fatalf("moved to a path that is not there: %q", out.String())
	}
	// Lowercased: the wording is the dialect's and the bare Diagnostics
	// here capitalizes it. What matters is *which* reason was reported.
	got := strings.ToLower(errs.String())
	if !strings.Contains(got, "no such file or directory") {
		t.Errorf("said %q, want the missing-directory reason", errs.String())
	}
	if strings.Contains(got, "not a directory") {
		t.Errorf("said %q, want the missing-directory reason and not the link refusal", errs.String())
	}
}

// A shell that has not got the letter never reaches any of this: the word
// falls to the unknown-letter question, which is where it was before the
// letter existed. Both answers of that question are checked, because the
// letter must not have quietly become an option for everybody.
func TestCdSymlinkFreeIsNotAnOptionWithoutTheAxis(t *testing.T) {
	for _, c := range []struct {
		name    string
		refuses Answer
	}{
		{"a shell that refuses an unknown letter", Yes},
		{"a shell that reads it as a directory", No},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, _ := cdTree(t)
			out, errs := &strings.Builder{}, &strings.Builder{}
			sem := PosixSemantics()
			sem.CdHasSymlinkFreeOption = No
			sem.CdRefusesUnknownOption = c.refuses
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Dir: dir,
				Stdout: out, Stderr: errs,
			})
			runCd(t, r, "cd -s real\necho st=$?\n")
			if strings.Contains(out.String(), "st=0\n") {
				t.Fatalf("`cd -s real` succeeded without the letter: %q", out.String())
			}
			if r.Dir != dir {
				t.Errorf("moved to %q without the letter", r.Dir)
			}
		})
	}
}
