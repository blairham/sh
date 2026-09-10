// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The inward half of the descriptor seam: a registered builtin that has opened
// something and needs the script to be able to reach it by number.
//
// It is here because a builtin found it missing. Everything else about a
// descriptor in this package goes the other way — a redirection puts a file in
// the table and SystemDescriptor asks what the kernel calls it — so a builtin
// that opened a socket had an *os.File, a script that wanted to write to it,
// and no way to join the two.

// descriptorSeamRunner is seamRunner with the one axis a published descriptor
// makes a script ask about: whether `>&10` is a duplication or an error.
//
// The number this seam hands out is deliberately clear of the single digits,
// so a script writing through it writes a two-digit target — and whether that
// is allowed is a thing the shells disagree about, which an unanswered axis
// reports rather than guesses. Answered here so the test asserts on the seam
// and not on the axis.
func descriptorSeamRunner(t *testing.T, out, errs *strings.Builder) *Runner {
	t.Helper()
	r := seamRunner(t, out, errs)
	r.Semantics.MultiDigitDuplicationTargetIsAnError = No
	return r
}

// The number comes back, and the file behind it is the one that was handed
// over: a builtin publishes a file, the script writes through the number, and
// the bytes are in the file.
//
// The write is the whole test. A seam that recorded the number and lost the
// file would answer this correctly right up to the last line.
func TestOpenDescriptorGivesTheScriptANumberItCanWriteThrough(t *testing.T) {
	var out, errs strings.Builder
	r := descriptorSeamRunner(t, &out, &errs)
	path := filepath.Join(t.TempDir(), "written")
	r.Register("publish", func(rr *Runner, _ context.Context, _ []string) int {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		rr.SetVar("FD", strconv.Itoa(rr.OpenDescriptor(f)))
		return 0
	})
	runSeam(t, r, "publish; echo through >&$FD; exec {FD}>&-")
	if out.String() != "" || errs.String() != "" {
		t.Fatalf("output = %q, errors = %q, want neither", out.String(), errs.String())
	}
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(text) != "through\n" {
		t.Errorf("the published file holds %q, want %q", text, "through\n")
	}
}

// The number a builtin chooses is the number the script gets, which is what a
// command taking `-u fd` or `-d fd` needs. And the two questions agree
// afterwards: the shell's table has it, so SystemDescriptor answers for it.
func TestSetDescriptorPutsAFileAtTheNumberItWasGiven(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	path := filepath.Join(t.TempDir(), "chosen")
	r.Register("publish", func(rr *Runner, _ context.Context, _ []string) int {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		rr.SetDescriptor(6, f)
		if _, known := rr.SystemDescriptor(6); !known {
			t.Error("the number the builtin chose is not one of the shell's")
		}
		return 0
	})
	runSeam(t, r, "publish; echo chosen >&6; exec 6>&-")
	if out.String() != "" || errs.String() != "" {
		t.Fatalf("output = %q, errors = %q, want neither", out.String(), errs.String())
	}
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(text) != "chosen\n" {
		t.Errorf("the chosen number holds %q, want %q", text, "chosen\n")
	}
}

// **A published descriptor is well clear of the single digits.** A script
// addresses itself by small numbers — `exec 3<file`, `2>&1` — and a builtin
// that handed one of those out would take a number the script may already be
// using and may be about to use.
func TestOpenDescriptorChoosesANumberAScriptDoesNotAddressItselfBy(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	r.Register("publish", func(rr *Runner, _ context.Context, _ []string) int {
		f, err := os.Create(filepath.Join(t.TempDir(), "n"))
		if err != nil {
			t.Fatal(err)
		}
		if fd := rr.OpenDescriptor(f); fd < 10 {
			t.Errorf("published descriptor %d, want one clear of the single digits", fd)
		}
		return 0
	})
	runSeam(t, r, "publish")
}
