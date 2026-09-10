// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"os/exec"
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

// **A descriptor kept from a child is closed there, and its neighbor is not.**
// The seam `sysopen -o cloexec` needs, and the pair rather than one of them:
// a table that handed nothing to a child would pass the first half alone, and
// one that handed everything over would pass the second.
//
// The child is `sh` because the question is what an *exec* sees. Nothing in
// this process can answer it: the runtime opens every file close-on-exec, so
// the descriptor a child inherits is the one this table rebuilds for it by
// hand, and the only way to read that back is to run something.
func TestKeepDescriptorFromChildrenClosesItThereAndLeavesTheRestOpen(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("no sh to run as a child: %v", err)
	}
	var out, errs strings.Builder
	r := descriptorSeamRunner(t, &out, &errs)
	dir := t.TempDir()
	kept, given := filepath.Join(dir, "kept"), filepath.Join(dir, "given")
	// Single-digit numbers, chosen rather than allocated. `>&10` is not a
	// duplication in every `sh` a machine may have — the oldest one here reads
	// a two-digit word after `>&` as a *filename* — so a probe written with
	// an allocated number measures the child's parser instead of this table.
	r.Register("publish", func(rr *Runner, _ context.Context, _ []string) int {
		for fd, path := range map[int]string{6: kept, 7: given} {
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			rr.SetDescriptor(fd, f)
			if path == kept {
				rr.KeepDescriptorFromChildren(fd)
			}
		}
		return 0
	})
	// The *fact* of the failure rather than the number: a redirection onto a
	// descriptor that is not there is 1 in one `sh` and 2 in another, and
	// neither is this package's to promise. The files below are the evidence
	// that cannot move.
	runSeam(t, r, "publish\n"+sh+" -c 'echo x >&6' 2>/dev/null\necho hidden=$(( $? != 0 ))\n"+
		sh+" -c 'echo y >&7' 2>/dev/null\necho shown=$(( $? != 0 ))\nexec 6>&- 7>&-")
	if got := out.String(); got != "hidden=1\nshown=0\n" {
		t.Errorf("statuses = %q, want %q", got, "hidden=1\nshown=0\n")
	}
	if text, err := os.ReadFile(kept); err != nil || len(text) != 0 {
		t.Errorf("the kept file holds %q (err %v), want it empty", text, err)
	}
	if text, err := os.ReadFile(given); err != nil || string(text) != "y\n" {
		t.Errorf("the given file holds %q (err %v), want %q", text, err, "y\n")
	}
}

// **The mark comes off when the number is written again.** A number is the
// script's to reuse, and a mark left on a descriptor that has been replaced
// would close a child's view of a file nobody asked to hide.
func TestKeepDescriptorFromChildrenIsForgottenWhenTheNumberIsReused(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("no sh to run as a child: %v", err)
	}
	var out, errs strings.Builder
	r := descriptorSeamRunner(t, &out, &errs)
	path := filepath.Join(t.TempDir(), "second")
	r.Register("publish", func(rr *Runner, _ context.Context, _ []string) int {
		first, err := os.Create(filepath.Join(t.TempDir(), "first"))
		if err != nil {
			t.Fatal(err)
		}
		rr.SetDescriptor(6, first)
		rr.KeepDescriptorFromChildren(6)
		second, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		rr.SetDescriptor(6, second)
		return 0
	})
	runSeam(t, r, "publish\n"+sh+" -c 'echo reused >&6' 2>/dev/null\necho st=$(( $? != 0 ))\nexec 6>&-")
	if got := out.String(); got != "st=0\n" {
		t.Errorf("status = %q, want %q", got, "st=0\n")
	}
	if text, err := os.ReadFile(path); err != nil || string(text) != "reused\n" {
		t.Errorf("the reused number holds %q (err %v), want %q", text, err, "reused\n")
	}
}

// **The number goes where a name says, including into an element.** The seam
// `sysopen -u name` needs, and the element is the half a builtin doing its own
// SetVar would get wrong: it would invent a parameter literally called `h[k]`
// and the script would find nothing under the key it wrote.
func TestSetDescriptorVariableResolvesANameTheWayARedirectionDoes(t *testing.T) {
	var out, errs strings.Builder
	r := descriptorSeamRunner(t, &out, &errs)
	path := filepath.Join(t.TempDir(), "keyed")
	r.Register("publish", func(rr *Runner, _ context.Context, _ []string) int {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		rr.SetDescriptorVariable("h[k]", rr.OpenDescriptor(f))
		return 0
	})
	// `${h[k]}` is also a *pattern*, and whether an unmatched one is an error
	// is a thing the shells disagree about. Answered here so the case asserts
	// on the seam rather than reporting an axis that decides nothing for it.
	r.Semantics.GlobNoMatchIsError = No
	runSeam(t, r, "typeset -A h\npublish\necho keyed >&${h[k]}")
	if out.String() != "" || errs.String() != "" {
		t.Fatalf("output = %q, errors = %q, want neither", out.String(), errs.String())
	}
	if text, err := os.ReadFile(path); err != nil || string(text) != "keyed\n" {
		t.Errorf("the keyed descriptor holds %q (err %v), want %q", text, err, "keyed\n")
	}
}

// **ReaderForFd is the stream, not the number.** A builtin reading "from
// descriptor n" gets what a redirection put there — the pipe of a pipeline on
// 0, a file the script opened above it — and a number nothing is open at is
// not a stream.
func TestReaderForFdIsWhateverTheTablePutAtTheNumber(t *testing.T) {
	var out, errs strings.Builder
	r := descriptorSeamRunner(t, &out, &errs)
	path := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(path, []byte("from-the-table"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.Register("take", func(rr *Runner, _ context.Context, args []string) int {
		fd, err := strconv.Atoi(args[0])
		if err != nil {
			t.Fatal(err)
		}
		in, ok := rr.ReaderForFd(fd)
		if !ok {
			rr.SetVar("GOT", "<none>")
			return 1
		}
		buf := make([]byte, 64)
		n, _ := in.Read(buf)
		rr.SetVar("GOT", string(buf[:n]))
		return 0
	})
	runSeam(t, r, "exec 6<"+path+"\ntake 6\necho \"six=[$GOT]\"\ntake 9\necho \"nine=[$GOT] st=$?\"\nexec 6<&-")
	want := "six=[from-the-table]\nnine=[<none>] st=1\n"
	if got := out.String(); got != want {
		t.Errorf("reads = %q, want %q", got, want)
	}
}
