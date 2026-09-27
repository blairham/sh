// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// When a declaration utility's array-literal operand is expanded, against when
// the command's own redirections are opened — interp/arrayoperandorder.go and
// #3806, where the elements were expanded inside the redirections and a
// substitution's standard error went wherever the command sent it.
//
// Named for the position and not for a shell: there is no axis here, because
// the two columns that have the construct and can be asked answer alike. The
// measurement is in the file under test.
//
// Every assertion below is written so that moving the expansion back inside
// the redirections fails it, and so that a "fix" that merely stopped applying
// the redirection fails it too — which is what the SWALLOWED line in each
// probe is for.

// operandOrderRun runs a script with an empty scratch directory and returns
// both streams, because the whole question is which of them a substitution's
// output reached.
func operandOrderRun(t *testing.T, dir, src string) (out, errOut string) {
	t.Helper()
	return operandOrderRunWith(t, dir, src, nil)
}

// operandOrderRunWith is the same with a hand on the vector, for the one
// control below whose shape asks an axis of its own.
func operandOrderRunWith(t *testing.T, dir, src string, set func(*Semantics)) (out, errOut string) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.DeclarationCommandWord = DeclarationByUnquotedLiteralWord
	if set != nil {
		set(&sem)
	}
	var o, e bytes.Buffer
	r := newTestRunner(t, &Runner{
		Dir: dir, Stdout: &o, Stderr: &e,
		Semantics: &sem, Name: "sh", Env: testPATH(),
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return o.String(), e.String()
}

// TestAnArrayOperandIsExpandedOutsideTheCommandsRedirections is the row #3806
// measured: the substitution in the operand runs before the command opens
// anything, so what it writes to standard error is not caught by the
// command's own `2>`.
//
// The second line is the control that makes the first one mean something. It
// sends a byte to standard error through the same redirection and must lose
// it: without that row a shell that had stopped applying redirections
// altogether would pass the assertion above it.
func TestAnArrayOperandIsExpandedOutsideTheCommandsRedirections(t *testing.T) {
	out, errOut := operandOrderRun(t, t.TempDir(),
		"typeset a=($(echo VISIBLE >&2)) 2>/dev/null\n"+
			"{ echo SWALLOWED >&2; } 2>/dev/null\n"+
			"echo done\n")
	if errOut != "VISIBLE\n" {
		t.Errorf("stderr = %q, want %q", errOut, "VISIBLE\n")
	}
	if out != "done\n" {
		t.Errorf("stdout = %q, want %q", out, "done\n")
	}
}

// TestAnArrayOperandStoresWhatItExpandedTo is the value control. The elements
// are computed in front of the command and consumed by the store that runs
// after the utility, so moving the expansion must not change a byte of what
// the name comes to hold — including the count, which is what says the list
// was split rather than handed over whole.
func TestAnArrayOperandStoresWhatItExpandedTo(t *testing.T) {
	const src = "typeset a=($(echo one; echo two) $(echo three >&2)) 2>/dev/null\n" +
		"{ echo SWALLOWED >&2; } 2>/dev/null\n" +
		`printf "[%s]" "${a[@]}"` + "\n"
	out, errOut := operandOrderRun(t, t.TempDir(), src)
	if out != "[one][two]" {
		t.Errorf("stored = %q, want %q", out, "[one][two]")
	}
	if errOut != "three\n" {
		t.Errorf("stderr = %q, want %q", errOut, "three\n")
	}
}

// TestAnArrayOperandIsExpandedExactlyOnce is the trap the position opens:
// expanding in front of the command and then letting the store expand again
// runs every side effect twice, and a substitution appending to a file is the
// only probe that can tell one run from two — the value is identical either
// way.
//
// The redirection is on the command as well, so this is the same shape as the
// test above rather than a quieter one.
func TestAnArrayOperandIsExpandedExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	out, _ := operandOrderRun(t, dir,
		"typeset a=($(echo mark >>log)) 2>/dev/null\n"+
			"echo done\n")
	if out != "done\n" {
		t.Errorf("stdout = %q, want %q", out, "done\n")
	}
	b, err := os.ReadFile(filepath.Join(dir, "log"))
	if err != nil {
		t.Fatalf("reading the log the substitution appended to: %v", err)
	}
	if got := strings.Count(string(b), "mark"); got != 1 {
		t.Errorf("the operand's substitution ran %d times, want 1: log = %q", got, b)
	}
}

// TestAScalarOperandIsExpandedOutsideTheRedirectionsToo is the neighbor that
// says the array operand was the only thing out of position. An ordinary
// assignment-shaped operand is an argument word and has always been expanded
// with the rest of them; it must still be, or a change that moved the array
// list would read as a change to where operands are expanded in general.
func TestAScalarOperandIsExpandedOutsideTheRedirectionsToo(t *testing.T) {
	out, errOut := operandOrderRun(t, t.TempDir(),
		"typeset s=$(echo VISIBLE >&2) 2>/dev/null\n"+
			"{ echo SWALLOWED >&2; } 2>/dev/null\n"+
			"echo done\n")
	if errOut != "VISIBLE\n" {
		t.Errorf("stderr = %q, want %q", errOut, "VISIBLE\n")
	}
	if out != "done\n" {
		t.Errorf("stdout = %q, want %q", out, "done\n")
	}
}

// TestABareArrayLiteralKeepsItsOwnExpansionPosition is the other control, and
// it is the one that pins the *shape* rather than the stream. A literal
// written as a statement of its own is not an operand — nothing runs between
// its expansion and its store — so the value it comes to must not move,
// however the operand is moved.
//
// The **stream** is a different question and one the panel splits on, which is
// why the axis is answered here rather than left to the vector: a command with
// no command word opens its redirections either before or after its
// assignments expand, and where it opens them first the substitution's
// standard error goes to the file the statement just opened. See
// interp/prefixredirorder.go. The value is `[kept]` at both answers, which is
// what this control is for.
func TestABareArrayLiteralKeepsItsOwnExpansionPosition(t *testing.T) {
	for _, row := range []struct {
		order PrefixRedirectionOrder
		errs  string
	}{
		{PrefixExpandedBeforeRedirectionsAlways, "VISIBLE\n"},
		{PrefixExpandedBeforeRedirectionsNever, ""},
	} {
		out, errOut := operandOrderRunWith(t, t.TempDir(),
			"a=($(echo VISIBLE >&2) kept) 2>/dev/null\n"+
				`printf "[%s]" "${a[@]}"`+"\n",
			func(s *Semantics) { s.PrefixExpandedBeforeTheRedirections = row.order })
		if errOut != row.errs {
			t.Errorf("%v: stderr = %q, want %q", row.order, errOut, row.errs)
		}
		if out != "[kept]" {
			t.Errorf("%v: stored = %q, want %q", row.order, out, "[kept]")
		}
	}
}
