// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The front end's half of history expansion: a program is read one physical
// line at a time, through the expander, once it has asked for a list.
//
// Named by the axis rather than by a shell, which is the rule outside
// dialect/. One shell in the panel answers Yes to
// Semantics.HistoryExpansionInAScript and two that *have* an expander answer
// No, so both answers are exercised here from the same front end.

// historyShell is a shell with the two option names, a list to keep them in,
// and the axis handed in.
func historyShell(out, errs *strings.Builder, inAScript interp.Answer) driver.Shell {
	sem := interp.PosixSemantics()
	sem.HistoryExpansion = interp.Yes
	sem.HistoryExpansionInAScript = inAScript
	return driver.Shell{
		Name:        "testsh",
		Dialect:     syntax.Core(),
		Semantics:   sem,
		Diagnostics: interp.Diagnostics{},
		Stdout:      out,
		Stderr:      errs,
		Register: func(r *interp.Runner) {
			r.AddSetOptions("history", "histexpand")
			// A list of the front end's own, which is all the gate asks for.
			// A dialect keeps its in a shell array so that a subshell gets a
			// copy; that property is the dialect's to test and this is a
			// slice.
			var list []string
			r.SetHistoryStore(
				func(*interp.Runner) []string { return list },
				func(_ *interp.Runner, line string) { list = append(list, line) },
			)
			r.Register("showlist", func(r *interp.Runner, _ context.Context, _ []string) int {
				for i, entry := range list {
					_, _ = fmt.Fprintf(r.Out(), "%d[%s]\n", i+1, entry)
				}
				return 0
			})
		},
	}
}

const historyProgram = "set -o history\nset -o histexpand\necho one two three\necho !!\n"

// With the axis on, the reference is expanded and the expanded line is echoed
// to the script's standard error.
func TestAScriptExpandsWhereTheDialectSaysSo(t *testing.T) {
	var out, errs strings.Builder
	if code := driver.MainArgs(historyShell(&out, &errs, interp.Yes), []string{"testsh", "-c", historyProgram}); code != 0 {
		t.Fatalf("status %d (stderr %q)", code, errs.String())
	}
	if out.String() != "one two three\necho one two three\n" {
		t.Errorf("ran %q, want the reference expanded", out.String())
	}
	if errs.String() != "echo echo one two three\n" {
		t.Errorf("echoed %q, want the expanded line", errs.String())
	}
}

// With it off, the same program moves the same two states and expands
// nothing — which is what the two shells with an expander and no script route
// do, and what this front end did for everybody before the gate existed.
func TestAScriptWithTheAxisOffExpandsNothing(t *testing.T) {
	var out, errs strings.Builder
	if code := driver.MainArgs(historyShell(&out, &errs, interp.No), []string{"testsh", "-c", historyProgram}); code != 0 {
		t.Fatalf("status %d (stderr %q)", code, errs.String())
	}
	if out.String() != "one two three\n!!\n" {
		t.Errorf("ran %q, want the two characters left alone", out.String())
	}
	if errs.String() != "" {
		t.Errorf("said %q, want nothing", errs.String())
	}
}

// And with the axis on but the option never written, nothing is collected at
// all: the gate is what fills the list, and a program that never asks for one
// is read exactly as it was.
func TestAProgramThatNeverAsksKeepsAnEmptyList(t *testing.T) {
	var out, errs strings.Builder
	if code := driver.MainArgs(historyShell(&out, &errs, interp.Yes), []string{
		"testsh", "-c", "echo one two three\nshowlist\necho !!\n",
	}); code != 0 {
		t.Fatalf("status %d (stderr %q)", code, errs.String())
	}
	if out.String() != "one two three\n!!\n" {
		t.Errorf("ran %q, want an empty list and no expansion", out.String())
	}
}

// The list holds the **logical** command, however many physical lines it took
// — and the line that turned the list on is not in it, because the gate is
// built after that line has run.
func TestTheListHoldsWholeCommands(t *testing.T) {
	var out, errs strings.Builder
	if code := driver.MainArgs(historyShell(&out, &errs, interp.Yes), []string{
		"testsh", "-c", "set -o history\nif true\nthen\n  echo hi\nfi\nshowlist\n",
	}); code != 0 {
		t.Fatalf("status %d (stderr %q)", code, errs.String())
	}
	want := "hi\n1[if true; then   echo hi; fi]\n2[showlist]\n"
	if out.String() != want {
		t.Errorf("listed %q, want %q", out.String(), want)
	}
}

// A here-document's body is not expanded and its newlines survive into the
// entry, because a `;` there would be body text.
func TestAHereDocumentIsOneEntryAndIsNotExpanded(t *testing.T) {
	var out, errs strings.Builder
	if code := driver.MainArgs(historyShell(&out, &errs, interp.Yes), []string{
		"testsh", "-c", "set -o history\nset -o histexpand\ncat <<EOD\nx !! y\nEOD\nshowlist\n",
	}); code != 0 {
		t.Fatalf("status %d (stderr %q)", code, errs.String())
	}
	want := "x !! y\n1[set -o histexpand]\n2[cat <<EOD\nx !! y\nEOD\n]\n3[showlist]\n"
	if out.String() != want {
		t.Errorf("listed %q, want %q", out.String(), want)
	}
	if errs.String() != "" {
		t.Errorf("echoed %q, want nothing — a body line is not expanded", errs.String())
	}
}

// A reference the list cannot answer takes the line with it and not the
// program: the line before it ran, the line after it runs, and the status is
// the one the command before it left.
//
// Where the *wording* of the complaint is asserted is dialect/bash, which is
// the only place that has one — this front end is built with an empty
// Diagnostics on purpose, so that what is tested here is the route.
func TestAnUnansweredReferenceDropsOnlyItsOwnLine(t *testing.T) {
	var out, errs strings.Builder
	code := driver.MainArgs(historyShell(&out, &errs, interp.Yes), []string{
		"testsh", "-c", "set -o history\nset -o histexpand\nfalse\necho !nosuch\necho after $?\n",
	})
	if out.String() != "after 1\n" {
		t.Errorf("ran %q, want the dropped line missing and the status left where `false` put it", out.String())
	}
	if !strings.Contains(errs.String(), "!nosuch: event not found") {
		t.Errorf("said %q, want the reference named", errs.String())
	}
	if code != 0 {
		t.Errorf("status %d, want 0 — a dropped line does not fail the script", code)
	}
}

// Standard input is read through the same gate, which is the route that was
// already line at a time before any of this.
func TestAProgramOnStandardInputExpandsToo(t *testing.T) {
	var out, errs strings.Builder
	sh := historyShell(&out, &errs, interp.Yes)
	sh.Stdin = strings.NewReader(historyProgram)
	if code := driver.MainArgs(sh, []string{"testsh"}); code != 0 {
		t.Fatalf("status %d (stderr %q)", code, errs.String())
	}
	if out.String() != "one two three\necho one two three\n" {
		t.Errorf("ran %q, want the reference expanded", out.String())
	}
}
