// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// An assignment prefixed to a frozen name is refused, and the refusal is
// reported wherever the command it belongs to is dispatched — not only on the
// builtin path, which is the only path that ever stored the assignment and so
// the only one that ever met the refusal. A prefix to an external command or
// to a function was taken in silence, reporting 0 with nothing on stderr,
// where every shell in the reference panel complains (#1219).
//
// What the refusal *costs* is the three axes prefixrefusal_test.go covers; it
// is held at the answers that cost nothing here so that these rows are about
// the reporting alone. The whole rendered line is asserted rather than a
// substring, because the whole of this is a diagnostic, and a Contains cannot
// see a prefix that should not be there.

// readonlyPrefixRun runs src with the *plain* refusal fatal and the prefix's
// refusal costing nothing, which is bash's pair. The plain fatality is the
// answer that would show up as a changed control flow if the prefix paths had
// been wired through refuseReadonly instead of deciding for themselves.
func readonlyPrefixRun(t *testing.T, src string) (stdout, stderr string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.ReadonlyReassignmentFatal = Yes
	sem.ReadonlyReassignmentByDeclarationFatal = Yes
	sem.PrefixToARegularBuiltinIsRefused = Yes
	sem.PrefixRefusalFatality = PrefixRefusalNeverFatal
	sem.PrefixRefusalCostsTheCommand = No
	// The order the check runs in relative to the command's values and
	// redirections is its own axis and is not what these rows are about;
	// answered the way three of the four presets do so the reports below
	// are the refusal's alone. TestWhenAFrozenPrefixIsChecked asserts both.
	sem.PrefixToAFrozenNameIsCheckedFirst = No
	var out, errs bytes.Buffer
	dir := t.TempDir()
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Diagnostics: &Diagnostics{
			Location:         LocationLineWord,
			ReadonlyVariable: "%s: readonly variable",
			NotFound:         "%[1]s: command not found",
		},
		Dir: dir, Name: "testsh",
		Vars: map[string]string{"PATH": "/bin:/usr/bin"},
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v\nstderr: %s", src, rerr, errs.String())
	}
	return out.String(), errs.String(), st
}

func TestAPrefixToAFrozenNameOnAnExternalCommandIsReported(t *testing.T) {
	out, errs, st := readonlyPrefixRun(t,
		"readonly x=1\nx=2 /bin/echo RAN\necho after")
	if want := "testsh: line 2: x: readonly variable\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
	// The command still runs and the list carries on, exactly as before the
	// refusal was reported at all: this change adds the complaint and moves
	// nothing else.
	if want := "RAN\nafter\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0 — the fatality axis must not have been consulted", st)
	}
}

func TestAPrefixToAFrozenNameOnAFunctionIsReported(t *testing.T) {
	out, errs, st := readonlyPrefixRun(t,
		"readonly x=1\nf() { echo INFUNC; }\nx=2 f\necho after")
	if want := "testsh: line 3: x: readonly variable\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
	if want := "INFUNC\nafter\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

func TestAPrefixToAFrozenNameOnAMissingCommandIsReportedFirst(t *testing.T) {
	_, errs, _ := readonlyPrefixRun(t,
		"readonly x=1\nx=2 nosuchcmd_zz\necho after")
	want := "testsh: line 2: x: readonly variable\n" +
		"testsh: line 2: nosuchcmd_zz: command not found\n"
	if errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
}

func TestARefusedPrefixIsNotHandedToTheChild(t *testing.T) {
	// Exported first, because that is the only spelling where the child's
	// environment tells a refused assignment from an absent one: it must see
	// the value the name still holds and never the one it was refused.
	out, errs, _ := readonlyPrefixRun(t,
		"export x=1\nreadonly x\nx=2 /usr/bin/printenv x")
	if want := "testsh: line 3: x: readonly variable\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
	if want := "1\n"; out != want {
		t.Errorf("stdout = %q, want %q — the child must not be handed the refused value", out, want)
	}
}

func TestAnUnfrozenNameBesideAFrozenOneStillReachesTheChild(t *testing.T) {
	// The refusal is one name's and not the prefix list's.
	out, errs, _ := readonlyPrefixRun(t,
		"readonly x=1\nx=2 y=3 /usr/bin/printenv y")
	if want := "testsh: line 2: x: readonly variable\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
	if want := "3\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestEveryFrozenNameInOnePrefixIsReported(t *testing.T) {
	// A shell that carries on names them all, in written order; naming only
	// the first is what the shells that abandon the command do, and
	// abandoning is the question this leaves alone.
	_, errs, _ := readonlyPrefixRun(t,
		"readonly x=1 z=9\nx=2 z=8 /bin/echo RAN")
	want := "testsh: line 2: x: readonly variable\n" +
		"testsh: line 2: z: readonly variable\n"
	if errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
}

func TestEveryFrozenNameInAPrefixToAFunctionIsReported(t *testing.T) {
	// The same completeness on the other path, because the two paths refuse
	// separately and a loop that stops at the first refusal reads as correct
	// against a one-name probe.
	_, errs, _ := readonlyPrefixRun(t,
		"readonly x=1 z=9\nf() { echo INFUNC; }\nx=2 z=8 f")
	want := "testsh: line 3: x: readonly variable\n" +
		"testsh: line 3: z: readonly variable\n"
	if errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
}

func TestAnOrdinaryPrefixIsNotReported(t *testing.T) {
	// The control: a prefix to a name nothing froze complains about nothing,
	// which is what tells a broken harness from a finding.
	out, errs, st := readonlyPrefixRun(t,
		"x=1\nx=2 /bin/echo RAN\necho \"after x=[$x]\"")
	if errs != "" {
		t.Errorf("stderr = %q, want nothing", errs)
	}
	if want := "RAN\nafter x=[1]\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

func TestARefusedPrefixWaitsForItsValueToExpand(t *testing.T) {
	// Which of two errors a prefix reports is a dialect question and not this
	// change's: bash names the frozen name and never evaluates the
	// expression, and dash, ksh93 and zsh name the division and never
	// mention the name. Ours expands first, and this pins that it still does
	// — a refusal reported ahead of the expansion would add a second line
	// here that no shell in the panel writes.
	_, errs, _ := readonlyPrefixRun(t,
		"readonly x=1\nx=$((1/0)) /bin/echo RAN")
	if strings.Contains(errs, "readonly variable") {
		t.Errorf("stderr = %q, want no refusal ahead of the failed expansion", errs)
	}
	if errs == "" {
		t.Errorf("stderr is empty, want the expansion's own complaint")
	}
}

func TestAPrefixIsNeverRefusedAsADeclaration(t *testing.T) {
	// A prefix is an assignment and never a declaration, so it takes the
	// plain wording and never keeps a builtin's name in its location — even
	// when it is refused with a builtin on the record. Asked with a
	// Diagnostics that names `eval`, which is the only kind of builtin that
	// can have a command dispatched under it at all, and with a bracketed
	// builtin location, so a leaked name would be visible twice over.
	//
	// Two things hold it, and only one of them is in the refusal: `eval`
	// clears the marker before it runs anything, because what a borrowed
	// script reports is the script's. That makes the form this refusal
	// passes unobservable *today* — a mutant that refuses a prefix as a
	// declaration changes nothing — and this is the test that would see it
	// the moment the clearing stops being true.
	f, err := syntax.Parse("readonly x=1\neval 'x=2 /bin/echo RAN'", syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sem := permissive()
	// Answered so the one line asserted below is the refusal's, as in
	// readonlyPrefixRun — the order the check runs in is its own axis.
	sem.PrefixToAFrozenNameIsCheckedFirst = No
	var out, errs bytes.Buffer
	dir := t.TempDir()
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Diagnostics: &Diagnostics{
			Location:                      LocationLineWord,
			BuiltinLocation:               LocationBracketLine,
			ReadonlyVariable:              "%s: readonly variable",
			ReadonlyVariableInDeclaration: "%[2]s: %[1]s: readonly variable",
			ReadonlyRefusalNamesBuiltin:   map[string]bool{"eval": true},
		},
		Dir: dir, Name: "testsh",
		Vars: map[string]string{"PATH": "/bin:/usr/bin"},
	})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run: %v\nstderr: %s", rerr, errs.String())
	}
	// The line is the eval'd string's own, which is a separate question from
	// this one; what the assertion is for is the rest of the line.
	if got, want := errs.String(), "testsh: line 1: x: readonly variable\n"; got != want {
		t.Errorf("stderr = %q, want %q — the plain wording, with no builtin in it", got, want)
	}
}

func TestAnOperandOnAShadowedDeclarationUtilityIsNotAPrefix(t *testing.T) {
	// An array given to a declaration utility is an *operand* rather than a
	// prefix — `local a=(x y)` — and it reaches the command as the bare name,
	// so a function that shadows the utility is handed an argument and no
	// assignment is attempted at all. Refusing it here would complain about
	// something the shell is not doing.
	out, errs, _ := readonlyPrefixRun(t,
		"readonly a=1\nlocal() { echo \"INFUNC[$1]\"; }\nlocal a=(x y)\necho after")
	if errs != "" {
		t.Errorf("stderr = %q, want nothing — an operand is an argument, not a prefix", errs)
	}
	if want := "INFUNC[a]\nafter\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}
