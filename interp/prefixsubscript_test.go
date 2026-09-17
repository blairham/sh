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

// A subscripted assignment written as a command **prefix** — `a[1]=v cmd` —
// against the panel's four readings. Measured 2026-09-16, `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, from a script file with stdin closed, over
// `f() { :; }; a[1]=v f; echo "[${a[1]}]"`:
//
//	bash 5.3.20   `` `a[1]': not a valid identifier ``, then `[]`
//	bash 3.2.57   silent, `[]`, and a child is handed `a[1]=v` verbatim
//	ksh93u+       `[v]`
//	zsh 5.9.2     `[v]`
//	dash 0.5.12   `a[1]=v: not found` — the word is a command there
//
// This shell gave a fifth: it dropped the subscript and made a temporary
// scalar `a` (#3433). The suite pins the two readings a dialect chooses
// between, the take-back the two storing shells disagree about, and the one
// row they agree on.

func subscriptPrefixRun(t *testing.T, src string, sem Semantics) (out, errs string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var stdout, stderr bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &stdout, Stderr: &stderr, Semantics: &sem,
		Dir: t.TempDir(), Name: "testsh",
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v\nstderr: %s", src, rerr, stderr.String())
	}
	return stdout.String(), stderr.String(), st
}

// storesTheElement is the vector the two shells that take the word share,
// with the take-back answered so that nothing but the reading under test is
// unanswered.
func storesTheElement(taken Answer) Semantics {
	sem := permissive()
	sem.SubscriptedAssignmentPrefix = SubscriptedPrefixStoresTheElement
	sem.SubscriptedPrefixIsTakenBack = taken
	return sem
}

func TestASubscriptedPrefixStoresTheElementItNames(t *testing.T) {
	// A function, so the store is visible inside the command as well as
	// after it — and the first element is read alongside, because the
	// defect this pins was a *scalar* `a` standing in for the element it
	// dropped, and a scalar store over an array lands in its first element.
	sem := storesTheElement(No)
	out, errs, st := subscriptPrefixRun(t,
		`f() { echo "in [${a[0]}][${a[1]}]"; }; a=(p q); a[1]=v f; echo "after [${a[0]}][${a[1]}]"`, sem)
	if want := "in [p][v]\nafter [p][v]\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if errs != "" || st != 0 {
		t.Errorf("stderr = %q status %d, want nothing said at 0", errs, st)
	}
}

// The element and not the first element: a prefix that stored the value under
// the name would pass the row above for `a[1]` in a one-based reading and
// still be writing the wrong cell.
func TestASubscriptedPrefixLeavesTheOtherElementsAlone(t *testing.T) {
	sem := storesTheElement(No)
	out, _, _ := subscriptPrefixRun(t,
		`a=(p q r); a[1]=v eval ':'; echo "[${a[0]}][${a[1]}][${a[2]}]"`, sem)
	if want := "[p][v][r]\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

// `a[1]+=v` appends to the element, which is what the element store gives for
// free and a scalar join does not: joining the *name's* value and storing it
// under the subscript would leave `pv` in element 1 of `(p q r)` rather than
// `qv`.
func TestASubscriptedPrefixAppendJoinsTheElement(t *testing.T) {
	sem := storesTheElement(No)
	out, _, _ := subscriptPrefixRun(t,
		`a=(p q r); a[1]+=v eval ':'; echo "[${a[1]}]"`, sem)
	if want := "[qv]\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

// The subscript is an expression the store evaluates, the same as a written
// `a[$i]=v`, so a prefix cannot be right by treating the brackets as text.
func TestASubscriptedPrefixEvaluatesItsSubscript(t *testing.T) {
	sem := storesTheElement(No)
	out, _, _ := subscriptPrefixRun(t,
		`i=1; a=(p q r); a[$i+1]=v eval ':'; echo "[${a[1]}][${a[2]}]"`, sem)
	if want := "[q][v]\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

// The take-back axis, over a *regular builtin* — the one command kind the two
// storing shells part on. ksh93 gives the element back exactly where it gives
// a scalar back; zsh gives back neither, because its scalar prefix outlives
// nothing at all and its element write outlives everything this shell runs.
func TestASubscriptedPrefixTakeBackFollowsItsAxis(t *testing.T) {
	for _, tc := range []struct {
		name  string
		taken Answer
		want  string
	}{
		{"given back where the dialect says so", Yes, "[q]\n"},
		{"kept where it says not", No, "[v]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := storesTheElement(tc.taken)
			out, errs, _ := subscriptPrefixRun(t,
				`a=(p q r); a[1]=v read -r _ignored </dev/null; echo "[${a[1]}]"`, sem)
			if out != tc.want {
				t.Errorf("stdout = %q, want %q (stderr %q)", out, tc.want, errs)
			}
		})
	}
}

// And the axis reaches the *function* route too, which has a persistence
// question of its own: under the keeping answer the element outlives the call
// even where AssignmentPrefixPersistsAfterAFunction says a prefix does not.
func TestASubscriptedPrefixIsKeptPastAFunctionWhereTheAxisSaysSo(t *testing.T) {
	sem := storesTheElement(No)
	sem.AssignmentPrefixPersistsAfterAFunction = No
	out, _, _ := subscriptPrefixRun(t,
		`f() { :; }; a=(p q r); a[1]=v f; echo "[${a[1]}]"`, sem)
	if want := "[v]\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

// The axis is asked only of a prefix that carries a subscript. A scalar one
// beside it must reach the same place with the question never put, or an
// unanswered vector would refuse on an ordinary line.
func TestTheTakeBackAxisIsNotAskedOfAScalarPrefix(t *testing.T) {
	sem := permissive()
	sem.SubscriptedPrefixIsTakenBack = Unspecified
	out, errs, _ := subscriptPrefixRun(t,
		`v=1; v=2 read -r _ignored </dev/null; echo "[$v]"`, sem)
	if out != "[1]\n" || errs != "" {
		t.Errorf("stdout = %q stderr = %q, want [1] with nothing said", out, errs)
	}
}

// The refusing reading: the word is named, nothing is assigned, and **the
// command runs anyway** at its own status.
func TestARefusedSubscriptedPrefixNamesTheWordAndRunsTheCommand(t *testing.T) {
	sem := permissive()
	sem.SubscriptedAssignmentPrefix = SubscriptedPrefixIsRefused
	out, errs, st := subscriptPrefixRun(t,
		`f() { echo "in [${a[0]}][${a[1]}]"; return 3; }; a=(p q); a[1]=v f; echo "st=$? after [${a[0]}][${a[1]}]"`, sem)
	if want := "in [p][q]\nst=3 after [p][q]\n"; out != want {
		t.Errorf("stdout = %q, want %q — the command's own status, and no element moved", out, want)
	}
	if want := "testsh: `a[1]': not a valid identifier\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want the script to have carried on to 0", st)
	}
}

// Of the word and not of the prefix: every other entry of the same prefix is
// applied, so `w=5 a[1]=1 f` shows the body `w`. The one probe that can say so
// is a *function*, since a prefix to a regular builtin is taken back whatever
// happened to it.
func TestARefusedSubscriptedPrefixLeavesTheRestOfThePrefixAlone(t *testing.T) {
	sem := permissive()
	sem.SubscriptedAssignmentPrefix = SubscriptedPrefixIsRefused
	out, _, _ := subscriptPrefixRun(t,
		`f() { echo "in w=[$w]"; }; w=5 a[1]=1 f`, sem)
	if want := "in w=[5]\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

// A line each, in written order, for a prefix carrying more than one.
func TestEveryRefusedSubscriptedPrefixGetsALine(t *testing.T) {
	sem := permissive()
	sem.SubscriptedAssignmentPrefix = SubscriptedPrefixIsRefused
	_, errs, _ := subscriptPrefixRun(t, `f() { :; }; a[1]=1 b[2]=2 f`, sem)
	want := "testsh: `a[1]': not a valid identifier\n" +
		"testsh: `b[2]': not a valid identifier\n"
	if errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
}

// The subscript is quoted back **as written**, and the refusal costs no
// evaluation at all: neither the subscript nor the right-hand side runs. A
// refusal that expanded first would report `a[1]` here, and would print
// `side`.
func TestARefusedSubscriptedPrefixEvaluatesNothing(t *testing.T) {
	// Once for each route that applies a prefix, because each of them
	// expands a value on its own and each has to be kept from doing it here.
	// The substitution writes to standard error, which is the only place a
	// run of it can be seen: its output is the value, and the value goes
	// nowhere.
	for _, tc := range []struct{ route, cmd, out string }{
		{"a function", "f", "done\n"},
		{"a builtin", "eval :", "done\n"},
		{"an external", "/bin/echo ran", "ran\ndone\n"},
	} {
		t.Run(tc.route, func(t *testing.T) {
			sem := permissive()
			sem.SubscriptedAssignmentPrefix = SubscriptedPrefixIsRefused
			out, errs, _ := subscriptPrefixRun(t,
				`f() { :; }; i=1; a[$i]=$(echo side >&2) `+tc.cmd+`; echo "done"`, sem)
			if out != tc.out {
				t.Errorf("stdout = %q, want %q", out, tc.out)
			}
			if want := "testsh: `a[$i]': not a valid identifier\n"; errs != want {
				t.Errorf("stderr = %q, want %q — the value must not have run", errs, want)
			}
		})
	}
}

// A subscript that would not evaluate is the same sentence, which is the
// other half of "nothing is evaluated": an arithmetic refusal here is fatal
// in every column that raises one, so a shell reaching the expression at all
// would end the script instead of running the command.
func TestARefusedSubscriptedPrefixNeverReachesABadSubscript(t *testing.T) {
	sem := permissive()
	sem.SubscriptedAssignmentPrefix = SubscriptedPrefixIsRefused
	out, errs, _ := subscriptPrefixRun(t, `f() { :; }; a[$((1/0))]=v f; echo "done"`, sem)
	if want := "done\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if !strings.Contains(errs, "`a[$((1/0))]': not a valid identifier") {
		t.Errorf("stderr = %q, want the subscript quoted back unevaluated", errs)
	}
	if strings.Contains(errs, "division") || strings.Contains(errs, "divide") {
		t.Errorf("stderr = %q, want nothing about the division", errs)
	}
}

// The refusal stands ahead of the command's redirections, which is where bash
// puts it: `a[1]=v f >/nope/x` writes the identifier complaint and *then* the
// file's.
func TestARefusedSubscriptedPrefixIsReportedBeforeARedirection(t *testing.T) {
	sem := permissive()
	sem.SubscriptedAssignmentPrefix = SubscriptedPrefixIsRefused
	_, errs, _ := subscriptPrefixRun(t, `f() { :; }; a[1]=v f > /nope/nowhere/x`, sem)
	ident := strings.Index(errs, "not a valid identifier")
	file := strings.Index(errs, "nowhere")
	if ident < 0 || file < 0 || ident > file {
		t.Errorf("stderr = %q, want the identifier complaint before the file's", errs)
	}
}

// And the refused word is left out of the trace, where the entries beside it
// are written as usual.
func TestARefusedSubscriptedPrefixIsNotTraced(t *testing.T) {
	sem := permissive()
	sem.SubscriptedAssignmentPrefix = SubscriptedPrefixIsRefused
	_, errs, _ := subscriptPrefixRun(t, `f() { :; }; set -x; w=1 a[1]=v f`, sem)
	if strings.Contains(errs, "a[1]=v") {
		t.Errorf("stderr = %q, want no trace line for the refused word", errs)
	}
	if !strings.Contains(errs, "w=1") {
		t.Errorf("stderr = %q, want the entry beside it still traced", errs)
	}
}

// A command a **child** runs never sees it, under either reading and in both
// storing shells: an array reaches no child's environment, and the element is
// not this shell's to keep. Unanimous, so it is done in the routes rather
// than asked.
func TestASubscriptedPrefixIsUnseenByAnExternalCommand(t *testing.T) {
	for _, taken := range []Answer{Yes, No} {
		sem := storesTheElement(taken)
		out, _, _ := subscriptPrefixRun(t,
			`a=(p q r); a[1]=v /bin/echo ran; echo "[${a[1]}]"`, sem)
		if want := "ran\n[q]\n"; out != want {
			t.Errorf("taken=%v: stdout = %q, want %q", taken, out, want)
		}
	}
}

// Its value still expands on the way, which is measured rather than assumed:
// `a[1]=$(echo side >&2; echo v) /bin/echo` writes `side` in ksh93 and zsh and
// leaves the element where it was.
func TestASubscriptedPrefixToAnExternalStillExpandsItsValue(t *testing.T) {
	sem := storesTheElement(No)
	out, _, _ := subscriptPrefixRun(t,
		`a=(p q r); a[1]=$(echo side) /bin/echo ran; echo "[${a[1]}]"`, sem)
	if want := "ran\n[q]\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

// Through `command` before an external it is the same nothing, which is the
// route that would otherwise have stored it: `command` is handled as a
// builtin here and the loop that applies a prefix is the builtin route's.
func TestASubscriptedPrefixThroughCommandBeforeAnExternalIsUnseen(t *testing.T) {
	sem := storesTheElement(No)
	out, _, _ := subscriptPrefixRun(t,
		`a=(p q r); a[1]=v command /bin/echo ran; echo "[${a[1]}]"`, sem)
	if want := "ran\n[q]\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

// An unanswered reading refuses by name, once for the command, and the axis
// is not asked at all by a prefix with no subscript in it.
func TestAnUnansweredSubscriptedPrefixRefuses(t *testing.T) {
	sem := permissive()
	sem.SubscriptedAssignmentPrefix = SubscriptedPrefixReadingUnspecified
	_, errs, _ := subscriptPrefixRun(t, `f() { :; }; a[1]=v f`, sem)
	if !strings.Contains(errs, "a subscripted assignment written as a command prefix") {
		t.Errorf("stderr = %q, want the unanswered axis named", errs)
	}
}

func TestTheSubscriptedPrefixAxisIsNotAskedOfAPlainPrefix(t *testing.T) {
	sem := permissive()
	sem.SubscriptedAssignmentPrefix = SubscriptedPrefixReadingUnspecified
	out, errs, _ := subscriptPrefixRun(t, `f() { echo "[$v]"; }; v=9 f`, sem)
	if out != "[9]\n" || errs != "" {
		t.Errorf("stdout = %q stderr = %q, want [9] with nothing said", out, errs)
	}
}

// The wording is the dialect's. A vector with none falls back to the sentence
// in the code, which is what the default above asserts; this pins that a
// dialect can word it otherwise.
func TestTheSubscriptedPrefixRefusalTakesTheDialectsWording(t *testing.T) {
	sem := permissive()
	sem.SubscriptedAssignmentPrefix = SubscriptedPrefixIsRefused
	f, err := syntax.Parse(`f() { :; }; a[1]=v f`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Name: "testsh",
		Diagnostics: &Diagnostics{SubscriptedPrefixIsNotAName: "not an identifier: %[1]s"},
	})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatal(rerr)
	}
	if want := "testsh: not an identifier: a[1]\n"; errs.String() != want {
		t.Errorf("stderr = %q, want %q", errs.String(), want)
	}
}

// Nor is a child handed the name with the subscript dropped from it, which is
// what this shell did: `a[1]=v /usr/bin/env` showed the child `a=v`, where
// bash 5.3, ksh93 and zsh show it nothing at all. Under the refusing reading
// as well, since the word is refused before the environment is built.
func TestASubscriptedPrefixPutsNothingInAChildsEnvironment(t *testing.T) {
	for _, tc := range []struct {
		name string
		sem  Semantics
	}{
		{"where the element is stored", storesTheElement(No)},
		{"where the word is refused", func() Semantics {
			s := permissive()
			s.SubscriptedAssignmentPrefix = SubscriptedPrefixIsRefused
			return s
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The scalar beside it is the control: it says the child ran and
			// was handed this prefix, so a missing `a` is an absence and not
			// a command that never started.
			out, errs, _ := subscriptPrefixRun(t, `w=1 a[1]=v /usr/bin/env`, tc.sem)
			lines := strings.Split(out, "\n")
			hasW := false
			for _, l := range lines {
				if l == "w=1" {
					hasW = true
				}
				if strings.HasPrefix(l, "a=") || strings.HasPrefix(l, "a[") {
					t.Errorf("the child was handed %q", l)
				}
			}
			if !hasW {
				t.Errorf("stdout = %q stderr = %q, want the child run and handed w=1", out, errs)
			}
		})
	}
}
